import { describe, it, expect } from 'vitest';
import { extractJiraTicket, parseVersion, detectComponent } from '../get-fixedversion.js';

describe('extractJiraTicket', () => {
  it('should extract ticket from PR title', () => {
    expect(extractJiraTicket('TT-12345: Fix bug')).toBe('TT-12345');
  });

  it('should extract ticket from branch name', () => {
    expect(extractJiraTicket('feature/TT-12345-fix-bug')).toBe('TT-12345');
  });

  it('should extract ticket from middle of text', () => {
    expect(extractJiraTicket('Fix bug (TT-12345)')).toBe('TT-12345');
  });

  it('should handle different project keys', () => {
    expect(extractJiraTicket('ABC-999: Test')).toBe('ABC-999');
    expect(extractJiraTicket('PROJ-1: Test')).toBe('PROJ-1');
  });

  it('should return null for no ticket', () => {
    expect(extractJiraTicket('Fix bug without ticket')).toBe(null);
  });

  it('should return null for empty input', () => {
    expect(extractJiraTicket('')).toBe(null);
    expect(extractJiraTicket(null)).toBe(null);
  });

  it('should not match single letter project keys', () => {
    expect(extractJiraTicket('T-12345')).toBe(null);
  });
});

describe('detectComponent', () => {
  it('should detect TIB component', () => {
    expect(detectComponent('TIB 1.7.0')).toEqual(['tyk-identity-broker']);
  });

  it('should detect Tyk component', () => {
    expect(detectComponent('Tyk 5.8.1')).toEqual(['tyk', 'tyk-analytics', 'tyk-analytics-ui']);
    expect(detectComponent('Tyk Gateway 5.8.1')).toEqual(['tyk', 'tyk-analytics', 'tyk-analytics-ui']);
  });

  it('should detect Pump component', () => {
    expect(detectComponent('Pump 1.9.0')).toEqual(['tyk-pump']);
    expect(detectComponent('Tyk Pump 1.9.0')).toEqual(['tyk-pump']);
  });

  it('should detect MDCB component', () => {
    expect(detectComponent('MDCB 2.0.0')).toEqual(['tyk-sink']);
  });

  it('should return empty array for unknown prefix', () => {
    expect(detectComponent('Unknown 1.0.0')).toEqual([]);
    expect(detectComponent('1.0.0')).toEqual([]);
  });

  it('should handle case insensitivity', () => {
    expect(detectComponent('tib 1.7.0')).toEqual(['tyk-identity-broker']);
    expect(detectComponent('TYK 5.8.1')).toEqual(['tyk', 'tyk-analytics', 'tyk-analytics-ui']);
  });

  it('should handle empty or null input', () => {
    expect(detectComponent('')).toEqual([]);
    expect(detectComponent(null)).toEqual([]);
  });
});

describe('parseVersion', () => {
  it('should parse semantic version', () => {
    const result = parseVersion('5.8.1');
    expect(result).toEqual({
      major: 5,
      minor: 8,
      patch: 1,
      original: '5.8.1',
      component: []
    });
  });

  it('should parse version with v prefix', () => {
    const result = parseVersion('v5.8.1');
    expect(result.major).toBe(5);
    expect(result.minor).toBe(8);
    expect(result.patch).toBe(1);
  });

  it('should parse version with Tyk prefix', () => {
    const result = parseVersion('Tyk 5.8.1');
    expect(result.major).toBe(5);
    expect(result.component).toEqual(['tyk', 'tyk-analytics', 'tyk-analytics-ui']);
  });

  it('should parse version with Tyk Gateway prefix', () => {
    const result = parseVersion('Tyk Gateway 5.8.1');
    expect(result.major).toBe(5);
    expect(result.component).toEqual(['tyk', 'tyk-analytics', 'tyk-analytics-ui']);
  });

  it('should handle TIB version', () => {
    const result = parseVersion('TIB 1.7.0');
    expect(result.major).toBe(1);
    expect(result.minor).toBe(7);
    expect(result.patch).toBe(0);
    expect(result.component).toEqual(['tyk-identity-broker']);
  });

  it('should handle Pump version', () => {
    const result = parseVersion('Pump 1.9.0');
    expect(result.major).toBe(1);
    expect(result.component).toEqual(['tyk-pump']);
  });

  it('should handle MDCB version', () => {
    const result = parseVersion('MDCB 2.0.0');
    expect(result.major).toBe(2);
    expect(result.component).toEqual(['tyk-sink']);
  });

  it('should handle minor version only', () => {
    const result = parseVersion('5.8');
    expect(result.major).toBe(5);
    expect(result.minor).toBe(8);
    expect(result.patch).toBe(null);
  });

  it('should handle major version only', () => {
    const result = parseVersion('5');
    expect(result.major).toBe(5);
    expect(result.minor).toBe(null);
    expect(result.patch).toBe(null);
  });

  it('should return null for invalid version', () => {
    expect(parseVersion('invalid')).toBe(null);
    expect(parseVersion('not-a-version')).toBe(null);
    expect(parseVersion('')).toBe(null);
    expect(parseVersion(null)).toBe(null);
  });

  it('should preserve original version string', () => {
    const result = parseVersion('TIB 1.7.0');
    expect(result.original).toBe('TIB 1.7.0');
  });
});
