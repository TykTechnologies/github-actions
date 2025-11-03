import { describe, it, expect } from 'vitest';
import { parseRepo, COMMENT_MARKER } from '../add-pr-comment.js';

describe('parseRepo', () => {
  it('should parse valid repo string', () => {
    const result = parseRepo('TykTechnologies/tyk');
    expect(result).toEqual({
      owner: 'TykTechnologies',
      repo: 'tyk'
    });
  });

  it('should parse repo with different owner', () => {
    const result = parseRepo('octocat/hello-world');
    expect(result).toEqual({
      owner: 'octocat',
      repo: 'hello-world'
    });
  });

  it('should handle repo names with hyphens', () => {
    const result = parseRepo('TykTechnologies/tyk-identity-broker');
    expect(result).toEqual({
      owner: 'TykTechnologies',
      repo: 'tyk-identity-broker'
    });
  });

  it('should handle repo names with underscores', () => {
    const result = parseRepo('owner/repo_name');
    expect(result).toEqual({
      owner: 'owner',
      repo: 'repo_name'
    });
  });

  it('should throw error for invalid format - no slash', () => {
    expect(() => parseRepo('invalid-repo')).toThrow('Repository must be in format "owner/repo"');
  });

  it('should throw error for invalid format - multiple slashes', () => {
    expect(() => parseRepo('owner/repo/extra')).toThrow('Repository must be in format "owner/repo"');
  });

  it('should throw error for empty string', () => {
    expect(() => parseRepo('')).toThrow('Repository must be in format "owner/repo"');
  });

  it('should handle slash at beginning - empty owner', () => {
    const result = parseRepo('/repo');
    expect(result).toEqual({
      owner: '',
      repo: 'repo'
    });
  });

  it('should handle slash at end - empty repo', () => {
    const result = parseRepo('owner/');
    expect(result).toEqual({
      owner: 'owner',
      repo: ''
    });
  });
});

describe('COMMENT_MARKER', () => {
  it('should have correct marker value', () => {
    expect(COMMENT_MARKER).toBe('<!-- branch-suggestions -->');
  });
});
