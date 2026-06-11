import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { postReleaseComments } from '../post-release-commands.js';
import { createPRComment, listPRComments, parseRepo, getPullRequest } from '../github-api.js';

vi.mock('../github-api.js', () => ({
  createPRComment: vi.fn(),
  listPRComments: vi.fn(),
  parseRepo: vi.fn(),
  getPullRequest: vi.fn()
}));

describe('postReleaseComments', () => {
  const repository = 'TykTechnologies/tyk';
  const prNumber = 123;
  
  beforeEach(() => {
    vi.clearAllMocks();
    parseRepo.mockReturnValue({ owner: 'TykTechnologies', repo: 'tyk' });
    getPullRequest.mockResolvedValue({ base: { ref: 'master' } });
    // Mock console methods to avoid noise in tests
    vi.spyOn(console, 'log').mockImplementation(() => {});
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('should skip if no match results are found', async () => {
    await postReleaseComments(repository, prNumber, {});
    expect(createPRComment).not.toHaveBeenCalled();
    expect(listPRComments).not.toHaveBeenCalled();
  });

  it('should skip if no eligible release branches are found', async () => {
    const matchData = {
      matchResults: [
        {
          fixVersion: '5.8.1',
          branches: [
            { branch: 'master', priority: 'required' },
            { branch: 'optional-branch', priority: 'optional' }
          ]
        }
      ]
    };
    await postReleaseComments(repository, prNumber, matchData);
    expect(createPRComment).not.toHaveBeenCalled();
  });

  it('should post release comments for required and recommended branches', async () => {
    const matchData = {
      matchResults: [
        {
          fixVersion: '5.8.1',
          branches: [
            { branch: 'release-5.8.1', priority: 'required' },
            { branch: 'release-5.8', priority: 'recommended' },
            { branch: 'master', priority: 'required' }
          ]
        }
      ]
    };
    
    listPRComments.mockResolvedValue([]); // No existing comments
    createPRComment.mockResolvedValue({ id: 1 });

    await postReleaseComments(repository, prNumber, matchData);

    expect(listPRComments).toHaveBeenCalledTimes(1);
    expect(createPRComment).toHaveBeenCalledTimes(2);
    expect(createPRComment).toHaveBeenCalledWith(
      'TykTechnologies', 
      'tyk', 
      123, 
      expect.stringContaining('/release to release-5.8.1')
    );
    expect(createPRComment).toHaveBeenCalledWith(
      'TykTechnologies', 
      'tyk', 
      123, 
      expect.stringContaining('/release to release-5.8')
    );
  });

  it('should skip branches that already have a release comment', async () => {
    const matchData = {
      matchResults: [
        {
          fixVersion: '5.8.1',
          branches: [
            { branch: 'release-5.8.1', priority: 'required' },
            { branch: 'release-5.8', priority: 'recommended' }
          ]
        }
      ]
    };
    
    // Mock that release-5.8.1 already has a comment
    listPRComments.mockResolvedValue([
      { body: 'Some comment with auto-release marker <!-- auto-release: release-5.8.1 -->' }
    ]);

    await postReleaseComments(repository, prNumber, matchData);

    expect(listPRComments).toHaveBeenCalledTimes(1);
    expect(createPRComment).toHaveBeenCalledTimes(1);
    expect(createPRComment).toHaveBeenCalledWith(
      'TykTechnologies', 
      'tyk', 
      123, 
      expect.stringContaining('/release to release-5.8')
    );
  });

  it('should handle API errors for one branch and continue with others', async () => {
    const matchData = {
      matchResults: [
        {
          fixVersion: '5.8.1',
          branches: [
            { branch: 'release-5.8.1', priority: 'required' },
            { branch: 'release-5.8', priority: 'recommended' }
          ]
        }
      ]
    };
    
    listPRComments.mockResolvedValue([]);
    createPRComment.mockImplementation((owner, repo, pr, body) => {
      if (body.includes('release-5.8.1')) throw new Error('API Error');
      return { id: 2 };
    });

    await expect(postReleaseComments(repository, prNumber, matchData)).rejects.toThrow(
      'Failed to post release comments for required branches'
    );

    expect(createPRComment).toHaveBeenCalledTimes(2);
    expect(console.error).toHaveBeenCalledWith(expect.stringContaining('Failed to process release comment for release-5.8.1'));
  });

  it('should not throw if only recommended branch fails', async () => {
    const matchData = {
      matchResults: [
        {
          fixVersion: '5.8.1',
          branches: [
            { branch: 'release-5.8', priority: 'recommended' }
          ]
        }
      ]
    };
    
    listPRComments.mockResolvedValue([]);
    createPRComment.mockRejectedValue(new Error('API Error'));

    await postReleaseComments(repository, prNumber, matchData);

    expect(createPRComment).toHaveBeenCalledTimes(1);
    expect(console.error).toHaveBeenCalledWith(expect.stringContaining('Failed to process release comment for release-5.8'));
  });

  it('should not throw if a match result is missing branches array', async () => {
    const matchData = {
      matchResults: [
        {
          fixVersion: '5.8.1'
        }
      ]
    };
    
    await postReleaseComments(repository, prNumber, matchData);
    expect(createPRComment).not.toHaveBeenCalled();
  });

  it('should collect unique branches across multiple fix versions', async () => {
    const matchData = {
      matchResults: [
        {
          fixVersion: '5.8.1',
          branches: [{ branch: 'release-5.8', priority: 'required' }]
        },
        {
          fixVersion: '5.8.0',
          branches: [{ branch: 'release-5.8', priority: 'required' }]
        }
      ]
    };
    
    listPRComments.mockResolvedValue([]);

    await postReleaseComments(repository, prNumber, matchData);

    expect(listPRComments).toHaveBeenCalledTimes(1);
    expect(createPRComment).toHaveBeenCalledTimes(1);
  });

  it('should exclude the PR base branch dynamically (e.g. main)', async () => {
    const matchData = {
      matchResults: [
        {
          fixVersion: '5.8.1',
          branches: [
            { branch: 'release-5.8.1', priority: 'required' },
            { branch: 'main', priority: 'required' },
            { branch: 'master', priority: 'required' }
          ]
        }
      ]
    };
    
    getPullRequest.mockResolvedValue({ base: { ref: 'main' } });
    listPRComments.mockResolvedValue([]);
    createPRComment.mockResolvedValue({ id: 1 });

    await postReleaseComments(repository, prNumber, matchData);

    expect(createPRComment).toHaveBeenCalledTimes(2);
    expect(createPRComment).toHaveBeenCalledWith(
      'TykTechnologies', 
      'tyk', 
      123, 
      expect.stringContaining('/release to release-5.8.1')
    );
    expect(createPRComment).toHaveBeenCalledWith(
      'TykTechnologies', 
      'tyk', 
      123, 
      expect.stringContaining('/release to master')
    );
    expect(createPRComment).not.toHaveBeenCalledWith(
      'TykTechnologies', 
      'tyk', 
      123, 
      expect.stringContaining('/release to main')
    );
  });

  it('should throw an error if a branch name contains invalid/malicious characters', async () => {
    const matchData = {
      matchResults: [
        {
          fixVersion: '5.8.1',
          branches: [
            { branch: 'release-5.8\n/release to main', priority: 'required' }
          ]
        }
      ]
    };
    
    await expect(postReleaseComments(repository, prNumber, matchData)).rejects.toThrow(
      'Security check failed: Invalid branch name format'
    );
    expect(createPRComment).not.toHaveBeenCalled();
  });
});
