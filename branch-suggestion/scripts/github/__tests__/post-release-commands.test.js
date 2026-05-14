import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { postReleaseComments } from '../post-release-commands.js';
import { createPRComment, findCommentByMarker, parseRepo } from '../github-api.js';

vi.mock('../github-api.js', () => ({
  createPRComment: vi.fn(),
  findCommentByMarker: vi.fn(),
  parseRepo: vi.fn()
}));

describe('postReleaseComments', () => {
  const repository = 'TykTechnologies/tyk';
  const prNumber = 123;
  
  beforeEach(() => {
    vi.clearAllMocks();
    parseRepo.mockReturnValue({ owner: 'TykTechnologies', repo: 'tyk' });
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
    expect(findCommentByMarker).not.toHaveBeenCalled();
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
    
    findCommentByMarker.mockResolvedValue(null); // No existing comments
    createPRComment.mockResolvedValue({ id: 1 });

    await postReleaseComments(repository, prNumber, matchData);

    expect(findCommentByMarker).toHaveBeenCalledTimes(2);
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
    findCommentByMarker.mockImplementation((owner, repo, pr, marker) => {
      if (marker.includes('release-5.8.1')) return { id: 1 };
      return null;
    });

    await postReleaseComments(repository, prNumber, matchData);

    expect(findCommentByMarker).toHaveBeenCalledTimes(2);
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
    
    findCommentByMarker.mockResolvedValue(null);
    createPRComment.mockImplementation((owner, repo, pr, body) => {
      if (body.includes('release-5.8.1')) throw new Error('API Error');
      return { id: 2 };
    });

    await postReleaseComments(repository, prNumber, matchData);

    expect(createPRComment).toHaveBeenCalledTimes(2);
    expect(console.error).toHaveBeenCalledWith(expect.stringContaining('Failed to process release comment for release-5.8.1'));
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
    
    findCommentByMarker.mockResolvedValue(null);

    await postReleaseComments(repository, prNumber, matchData);

    expect(findCommentByMarker).toHaveBeenCalledTimes(1);
    expect(createPRComment).toHaveBeenCalledTimes(1);
  });
});
