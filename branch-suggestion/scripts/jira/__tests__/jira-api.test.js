import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Set env var before importing
process.env.JIRA_READ_AUTH = 'test-token';
process.env.JIRA_BASE_URL = 'https://api.atlassian.com/ex/jira/cloud-123';

import { extractJQL, jiraAPI, searchIssues, getIssue, formatIssue, getJiraApiBaseUrl, main } from '../jira-api.js';
import readline from 'readline';

// Mock fetch
global.fetch = vi.fn();

vi.mock('readline', () => ({
  default: {
    createInterface: vi.fn()
  }
}));

describe('extractJQL', () => {
  it('should extract JQL from a valid JIRA URL', () => {
    const url = 'https://tyktech.atlassian.net/jira/software/c/projects/TT/issues/?jql=project%20%3D%20TT%20AND%20status%20!%3D%20closed';
    expect(extractJQL(url)).toBe('project = TT AND status != closed');
  });

  it('should return input if it is not a URL but contains jql=', () => {
    const input = 'jql=project = TT';
    // URL parsing will fail, so it returns input
    expect(extractJQL(input)).toBe(input);
  });

  it('should return input if it is a direct JQL query', () => {
    const input = 'project = TT AND status != closed';
    expect(extractJQL(input)).toBe(input);
  });

  it('should return input if URL does not contain jql parameter', () => {
    const url = 'https://tyktech.atlassian.net/browse/TT-123';
    expect(extractJQL(url)).toBe(url);
  });
});

describe('jiraAPI', () => {
  const originalEnv = process.env;

  beforeEach(() => {
    vi.resetModules();
    process.env = { ...originalEnv };
    global.fetch.mockClear();
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  it('should throw error if JIRA_READ_AUTH is not set', async () => {
    delete process.env.JIRA_READ_AUTH;
    await expect(jiraAPI('/test')).rejects.toThrow('JIRA_READ_AUTH must be set');
  });

  it('should handle 401 Unauthorized error', async () => {
    global.fetch.mockResolvedValue({
      ok: false,
      status: 401,
      text: vi.fn().mockResolvedValue('Unauthorized')
    });

    await expect(jiraAPI('/test')).rejects.toThrow('JIRA API Error (401): Unauthorized');
  });

  it('should handle 404 Not Found error', async () => {
    global.fetch.mockResolvedValue({
      ok: false,
      status: 404,
      text: vi.fn().mockResolvedValue('Issue does not exist')
    });

    await expect(jiraAPI('/test')).rejects.toThrow('JIRA API Error (404): Issue does not exist');
  });

  it('should return JSON on success', async () => {
    const mockData = { key: 'TT-123' };
    global.fetch.mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(mockData)
    });

    const result = await jiraAPI('/test');
    expect(result).toEqual(mockData);
  });

  it('should throw error if JIRA_BASE_URL is not set', async () => {
    delete process.env.JIRA_BASE_URL;
    global.fetch.mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue({})
    });

    await expect(jiraAPI('/test')).rejects.toThrow('JIRA_BASE_URL must be set');
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it('should use JIRA_BASE_URL for scoped API token requests', async () => {
    global.fetch.mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue({})
    });

    await jiraAPI('/test');

    expect(global.fetch.mock.calls[0][0]).toBe('https://api.atlassian.com/ex/jira/cloud-123/rest/api/3/test');
  });

  it('should remove trailing slash from JIRA_BASE_URL', () => {
    process.env.JIRA_BASE_URL = 'https://api.atlassian.com/ex/jira/cloud-123/';
    expect(getJiraApiBaseUrl()).toBe('https://api.atlassian.com/ex/jira/cloud-123');
  });

  it('should require JIRA_BASE_URL when building Jira API base URL', () => {
    delete process.env.JIRA_BASE_URL;
    expect(() => getJiraApiBaseUrl()).toThrow('JIRA_BASE_URL must be set');
  });
});

describe('searchIssues', () => {
  beforeEach(() => {
    global.fetch.mockClear();
    process.env.JIRA_BASE_URL = 'https://api.atlassian.com/ex/jira/cloud-123';
  });

  it('should call jiraAPI with correct parameters', async () => {
    global.fetch.mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue({ issues: [] })
    });

    await searchIssues('project = TT', 10, 20);

    expect(global.fetch).toHaveBeenCalledTimes(1);
    const url = global.fetch.mock.calls[0][0];
    expect(url).toContain('/search/jql?');
    expect(url).toContain('jql=project+%3D+TT');
    expect(url).toContain('startAt=10');
    expect(url).toContain('maxResults=20');
  });
});

describe('getIssue', () => {
  beforeEach(() => {
    global.fetch.mockClear();
    process.env.JIRA_BASE_URL = 'https://api.atlassian.com/ex/jira/cloud-123';
  });

  it('should request only fields needed for branch suggestions', async () => {
    global.fetch.mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue({ key: 'TT-123' })
    });

    await getIssue('TT-123');

    const url = global.fetch.mock.calls[0][0];
    expect(url).toContain('/issue/TT-123?');
    expect(url).toContain('fields=summary,priority,issuetype,fixVersions');
  });
});

describe('formatIssue', () => {
  it('should format issue correctly', () => {
    const issue = {
      key: 'TT-123',
      fields: {
        summary: 'Test issue',
        status: { name: 'In Progress' },
        issuetype: { name: 'Bug' },
        priority: { name: 'High' },
        created: '2023-01-01T00:00:00.000Z',
        assignee: { displayName: 'John Doe' },
        reporter: { displayName: 'Jane Doe' },
        labels: ['test', 'bug'],
        components: [{ name: 'API' }]
      }
    };

    const formatted = formatIssue(issue, 0);
    expect(formatted).toContain('1. [TT-123] Test issue');
    expect(formatted).toContain('Status: In Progress');
    expect(formatted).toContain('Type: Bug');
    expect(formatted).toContain('Priority: High');
    expect(formatted).toContain('Assignee: John Doe');
    expect(formatted).toContain('Reporter: Jane Doe');
    expect(formatted).toContain('Labels: test, bug');
    expect(formatted).toContain('Components: API');
    expect(formatted).toContain('Link: https://tyktech.atlassian.net/browse/TT-123');
  });

  it('should handle missing fields', () => {
    const issue = {
      key: 'TT-123',
      fields: {}
    };

    const formatted = formatIssue(issue, 0);
    expect(formatted).toContain('1. [TT-123] No summary');
    expect(formatted).toContain('Status: Unknown');
    expect(formatted).toContain('Type: Unknown');
    expect(formatted).toContain('Priority: None');
    expect(formatted).toContain('Created: Unknown');
  });
});

describe('main execution', () => {
  let originalArgv;
  let exitMock;
  let consoleLogMock;
  let consoleErrorMock;
  let stdoutWriteMock;

  beforeEach(() => {
    originalArgv = process.argv;
    exitMock = vi.spyOn(process, 'exit').mockImplementation(() => {});
    consoleLogMock = vi.spyOn(console, 'log').mockImplementation(() => {});
    consoleErrorMock = vi.spyOn(console, 'error').mockImplementation(() => {});
    stdoutWriteMock = vi.spyOn(process.stdout, 'write').mockImplementation(() => {});
    vi.clearAllMocks();
    global.fetch.mockClear();
  });

  afterEach(() => {
    process.argv = originalArgv;
    vi.restoreAllMocks();
  });

  it('should exit with code 1 if no arguments provided', async () => {
    process.argv = ['node', 'script.js'];

    await main();

    expect(consoleLogMock).toHaveBeenCalledWith(expect.stringContaining('Usage:'));
    expect(exitMock).toHaveBeenCalledWith(1);
  });

  it('should handle pagination when total > pageSize and user answers yes', async () => {
    process.argv = ['node', 'script.js', 'project = TT'];
    
    // Mock first page
    global.fetch.mockResolvedValueOnce({
      ok: true,
      json: vi.fn().mockResolvedValue({
        total: 100,
        issues: Array(50).fill({ key: 'TT-1', fields: { summary: 'Test' } })
      })
    });

    // Mock second page
    global.fetch.mockResolvedValueOnce({
      ok: true,
      json: vi.fn().mockResolvedValue({
        total: 100,
        issues: Array(50).fill({ key: 'TT-2', fields: { summary: 'Test 2' } })
      })
    });

    // Mock readline to answer 'yes'
    const questionMock = vi.fn((query, cb) => cb('yes'));
    readline.createInterface.mockReturnValue({
      question: questionMock,
      close: vi.fn()
    });

    await main();

    expect(global.fetch).toHaveBeenCalledTimes(2);
    expect(consoleLogMock).toHaveBeenCalledWith(expect.stringContaining('Displayed 100 of 100 total issues'));
  });

  it('should not fetch remaining pages if user answers no', async () => {
    process.argv = ['node', 'script.js', 'project = TT'];
    
    // Mock first page
    global.fetch.mockResolvedValueOnce({
      ok: true,
      json: vi.fn().mockResolvedValue({
        total: 100,
        issues: Array(50).fill({ key: 'TT-1', fields: { summary: 'Test' } })
      })
    });

    // Mock readline to answer 'no'
    const questionMock = vi.fn((query, cb) => cb('no'));
    readline.createInterface.mockReturnValue({
      question: questionMock,
      close: vi.fn()
    });

    await main();

    expect(global.fetch).toHaveBeenCalledTimes(1);
    expect(consoleLogMock).toHaveBeenCalledWith(expect.stringContaining('Displayed 50 of 100 total issues'));
  });
});
