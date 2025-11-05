import { describe, it, expect } from 'vitest';
import {
  generateBranchCandidates,
  filterFixVersionsByRepository,
  matchBranches,
  getBranchReason,
  getBranchPriority
} from '../match-branches.js';

describe('generateBranchCandidates', () => {
  it('should generate candidates for patch release', () => {
    const parsed = { major: 5, minor: 8, patch: 1 };
    const candidates = generateBranchCandidates(parsed);
    expect(candidates).toEqual(['release-5.8.1', 'release-5.8', 'release-5', 'master']);
  });

  it('should generate candidates for minor release', () => {
    const parsed = { major: 5, minor: 8, patch: 0 };
    const candidates = generateBranchCandidates(parsed);
    expect(candidates).toEqual(['release-5.8.0', 'release-5.8', 'release-5', 'master']);
  });

  it('should generate candidates for major release', () => {
    const parsed = { major: 5, minor: null, patch: null };
    const candidates = generateBranchCandidates(parsed);
    expect(candidates).toEqual(['release-5', 'master']);
  });

  it('should return only master for null input', () => {
    const candidates = generateBranchCandidates(null);
    expect(candidates).toEqual(['master']);
  });

  it('should remove duplicates', () => {
    const parsed = { major: 5, minor: 0, patch: 0 };
    const candidates = generateBranchCandidates(parsed);
    // Should not have duplicate 'release-5'
    expect(candidates.filter(c => c === 'release-5').length).toBe(1);
  });
});

describe('filterFixVersionsByRepository', () => {
  it('should filter versions by component - TIB', () => {
    const fixVersions = [
      { name: 'TIB 1.7.0', parsed: { component: ['tyk-identity-broker'] } },
      { name: 'Tyk 5.8.1', parsed: { component: ['tyk', 'tyk-analytics'] } },
      { name: '1.0.0', parsed: { component: [] } }
    ];

    const filtered = filterFixVersionsByRepository(fixVersions, 'TykTechnologies/tyk-identity-broker');
    expect(filtered).toHaveLength(2); // TIB version + no-component version
    expect(filtered[0].name).toBe('TIB 1.7.0');
    expect(filtered[1].name).toBe('1.0.0');
  });

  it('should filter versions by component - Tyk', () => {
    const fixVersions = [
      { name: 'TIB 1.7.0', parsed: { component: ['tyk-identity-broker'] } },
      { name: 'Tyk 5.8.1', parsed: { component: ['tyk', 'tyk-analytics'] } },
    ];

    const filtered = filterFixVersionsByRepository(fixVersions, 'TykTechnologies/tyk');
    expect(filtered).toHaveLength(1);
    expect(filtered[0].name).toBe('Tyk 5.8.1');
  });

  it('should include all versions with empty component', () => {
    const fixVersions = [
      { name: '1.0.0', parsed: { component: [] } },
      { name: '2.0.0', parsed: { component: [] } }
    ];

    const filtered = filterFixVersionsByRepository(fixVersions, 'TykTechnologies/any-repo');
    expect(filtered).toHaveLength(2);
  });

  it('should return all versions when no repository specified', () => {
    const fixVersions = [
      { name: 'TIB 1.7.0', parsed: { component: ['tyk-identity-broker'] } },
      { name: 'Tyk 5.8.1', parsed: { component: ['tyk'] } }
    ];

    const filtered = filterFixVersionsByRepository(fixVersions, null);
    expect(filtered).toHaveLength(2);
  });
});

describe('getBranchPriority', () => {
  it('should return required for master', () => {
    const priority = getBranchPriority('master', { parsed: { major: 5, minor: 8, patch: 1 } });
    expect(priority).toBe('required');
  });

  it('should return required for exact patch version branch', () => {
    const priority = getBranchPriority('release-5.8.1', { parsed: { major: 5, minor: 8, patch: 1 } });
    expect(priority).toBe('required');
  });

  it('should return required for patch release branch', () => {
    const priority = getBranchPriority('release-5.8', { parsed: { major: 5, minor: 8, patch: 1 } });
    expect(priority).toBe('required');
  });

  it('should return recommended for minor release branch', () => {
    const priority = getBranchPriority('release-5.8', { parsed: { major: 5, minor: 8, patch: 0 } });
    expect(priority).toBe('recommended');
  });

  it('should return recommended for major release branch', () => {
    const priority = getBranchPriority('release-5', { parsed: { major: 5, minor: 8, patch: 1 } });
    expect(priority).toBe('recommended');
  });
});

describe('getBranchReason', () => {
  it('should return reason for master branch', () => {
    const reason = getBranchReason('master', { parsed: { major: 5, minor: 8, patch: 1 } });
    expect(reason).toContain('Main development branch');
  });

  it('should return reason for exact patch version branch', () => {
    const reason = getBranchReason('release-5.8.1', {
      name: '5.8.1',
      parsed: { major: 5, minor: 8, patch: 1 }
    });
    expect(reason).toContain('Exact version branch');
    expect(reason).toContain('5.8.1');
  });

  it('should return reason for patch release', () => {
    const reason = getBranchReason('release-5.8', {
      name: '5.8.1',
      parsed: { major: 5, minor: 8, patch: 1 }
    });
    expect(reason).toContain('required for creating 5.8.1');
  });

  it('should return reason for minor release', () => {
    const reason = getBranchReason('release-5.8', {
      name: '5.8.0',
      parsed: { major: 5, minor: 8, patch: 0 }
    });
    expect(reason).toContain('5.8.x releases');
  });
});

describe('matchBranches', () => {
  it('should match fix versions to branches', () => {
    const fixVersions = [
      {
        name: '5.8.1',
        parsed: { major: 5, minor: 8, patch: 1, component: [] }
      }
    ];
    const repoBranches = [
      { name: 'master' },
      { name: 'release-5.8' },
      { name: 'release-5' }
    ];

    const results = matchBranches(fixVersions, repoBranches);
    expect(results).toHaveLength(1);
    expect(results[0].branches).toHaveLength(3);
    expect(results[0].branches[0].branch).toBe('release-5.8');
    expect(results[0].branches[0].priority).toBe('required');
  });

  it('should return warning when no release branches found', () => {
    const fixVersions = [
      { name: '5.8.1', parsed: { major: 5, minor: 8, patch: 1, component: [] } }
    ];
    const repoBranches = [
      { name: 'master' }
    ];

    const results = matchBranches(fixVersions, repoBranches);
    expect(results[0].warning).toBeDefined();
    expect(results[0].warning).toContain('Expected release branches not found');
  });

  it('should handle multiple fix versions', () => {
    const fixVersions = [
      { name: '5.8.1', parsed: { major: 5, minor: 8, patch: 1, component: [] } },
      { name: '5.9.0', parsed: { major: 5, minor: 9, patch: 0, component: [] } }
    ];
    const repoBranches = [
      { name: 'master' },
      { name: 'release-5.8' },
      { name: 'release-5.9' }
    ];

    const results = matchBranches(fixVersions, repoBranches);
    expect(results).toHaveLength(2);
  });

  it('should only match existing branches', () => {
    const fixVersions = [
      { name: '5.8.1', parsed: { major: 5, minor: 8, patch: 1, component: [] } }
    ];
    const repoBranches = [
      { name: 'master' },
      { name: 'release-5.7' } // Different version
    ];

    const results = matchBranches(fixVersions, repoBranches);
    expect(results[0].branches).toHaveLength(1);
    expect(results[0].branches[0].branch).toBe('master');
  });

  it('should prefer exact version match when available', () => {
    const fixVersions = [
      { name: '5.10.1', parsed: { major: 5, minor: 10, patch: 1, component: [] } }
    ];
    const repoBranches = [
      { name: 'master' },
      { name: 'release-5.10' },
      { name: 'release-5.10.1' }
    ];

    const results = matchBranches(fixVersions, repoBranches);
    expect(results[0].branches).toHaveLength(3);
    expect(results[0].branches[0].branch).toBe('release-5.10.1');
    expect(results[0].branches[0].priority).toBe('required');
    expect(results[0].branches[1].branch).toBe('release-5.10');
    expect(results[0].branches[1].priority).toBe('required');
    expect(results[0].branches[2].branch).toBe('master');
  });

  it('should fallback to minor version when exact match not available', () => {
    const fixVersions = [
      { name: '5.10.1', parsed: { major: 5, minor: 10, patch: 1, component: [] } }
    ];
    const repoBranches = [
      { name: 'master' },
      { name: 'release-5.10' }
    ];

    const results = matchBranches(fixVersions, repoBranches);
    expect(results[0].branches).toHaveLength(2);
    expect(results[0].branches[0].branch).toBe('release-5.10');
    expect(results[0].branches[0].priority).toBe('required');
  });
});
