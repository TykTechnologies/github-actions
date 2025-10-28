#!/usr/bin/env node
import dotenv from 'dotenv';
import { URL } from 'url';
import readline from 'readline';

// Only load .env if JIRA_EMAIL is not already set (to avoid log output in CI)
if (!process.env.JIRA_EMAIL) {
    dotenv.config();
}

// JIRA configuration
const JIRA_BASE_URL = 'https://tyktech.atlassian.net';
const JIRA_EMAIL = process.env.JIRA_EMAIL;
const JIRA_API_TOKEN = process.env.JIRA_API_TOKEN;

// Debug logging (without exposing sensitive data)
console.error('DEBUG: Environment check:');
console.error(`  JIRA_EMAIL: ${JIRA_EMAIL ? 'SET' : 'EMPTY'}`);
console.error(`  JIRA_API_TOKEN: ${JIRA_API_TOKEN ? 'SET' : 'EMPTY'}`);
console.error(`  All JIRA env vars: ${Object.keys(process.env).filter(k => k.includes('JIRA')).join(', ')}`);

// Extract JQL from URL or use directly
function extractJQL(input) {
  // Check if input is a URL
  if (input.includes('atlassian.net') || input.includes('jql=')) {
    try {
      const url = new URL(input);
      const jql = url.searchParams.get('jql');
      if (jql) {
        console.log('Extracted JQL from URL:', jql);
        return jql;
      }
    } catch (e) {
      // Not a valid URL, might be direct JQL
    }
  }
  return input;
}

// Make JIRA API request
async function jiraAPI(endpoint, options = {}) {
  if (!JIRA_EMAIL || !JIRA_API_TOKEN) {
    throw new Error('JIRA_EMAIL and JIRA_API_TOKEN must be set in .env file');
  }

  const auth = Buffer.from(`${JIRA_EMAIL}:${JIRA_API_TOKEN}`).toString('base64');
  
  const response = await fetch(`${JIRA_BASE_URL}/rest/api/3${endpoint}`, {
    ...options,
    headers: {
      'Authorization': `Basic ${auth}`,
      'Accept': 'application/json',
      'Content-Type': 'application/json',
      ...options.headers
    }
  });

  if (!response.ok) {
    const error = await response.text();
    throw new Error(`JIRA API Error (${response.status}): ${error}`);
  }

  return response.json();
}

// Search for issues using JQL
async function searchIssues(jql, startAt = 0, maxResults = 50) {
  // Use GET request with query parameters for v3 API
  const params = new URLSearchParams({
    jql: jql,
    startAt: startAt.toString(),
    maxResults: maxResults.toString(),
    fields: 'key,summary,status,issuetype,priority,created,assignee,reporter,customfield_10116,customfield_10117,customfield_10118,labels,components'
  });

  return jiraAPI(`/search/jql?${params}`, {
    method: 'GET'
  });
}

// Get issue details
async function getIssue(issueKey) {
  return jiraAPI(`/issue/${issueKey}`);
}

// Format issue for display
function formatIssue(issue, index) {
  const lines = [];
  lines.push(`${index + 1}. [${issue.key}] ${issue.fields?.summary || 'No summary'}`);
  lines.push(`   Status: ${issue.fields?.status?.name || 'Unknown'}`);
  lines.push(`   Type: ${issue.fields?.issuetype?.name || 'Unknown'}`);
  lines.push(`   Priority: ${issue.fields?.priority?.name || 'None'}`);
  lines.push(`   Created: ${issue.fields?.created ? new Date(issue.fields.created).toLocaleDateString() : 'Unknown'}`);
  
  if (issue.fields?.assignee) {
    lines.push(`   Assignee: ${issue.fields.assignee.displayName}`);
  }
  if (issue.fields?.reporter) {
    lines.push(`   Reporter: ${issue.fields.reporter.displayName}`);
    // Check if created via Zendesk integration
    if (issue.fields.reporter.displayName === 'Zendesk Support for Jira') {
      lines.push(`   Source: Zendesk (check Zendesk Support tab in JIRA)`);
    }
  }
  
  // Common custom fields for "Customers Impacted"
  const customFields = [
    issue.fields?.customfield_10116,
    issue.fields?.customfield_10117,
    issue.fields?.customfield_10118
  ].filter(Boolean);
  
  if (customFields.length > 0) {
    lines.push(`   Customers Impacted: Yes`);
  }
  
  if (issue.fields?.labels && issue.fields.labels.length > 0) {
    lines.push(`   Labels: ${issue.fields.labels.join(', ')}`);
  }
  
  if (issue.fields?.components && issue.fields.components.length > 0) {
    lines.push(`   Components: ${issue.fields.components.map(c => c.name).join(', ')}`);
  }
  
  lines.push(`   Link: ${JIRA_BASE_URL}/browse/${issue.key}`);
  
  return lines.join('\n');
}

async function main() {
  const args = process.argv.slice(2);
  
  if (args.length === 0) {
    console.log('Usage: node jira-api.js "<JQL query or JIRA URL>"');
    console.log('\nExamples:');
    console.log('  node jira-api.js "project = TT AND status != closed"');
    console.log('  node jira-api.js "https://tyktech.atlassian.net/jira/software/c/projects/TT/issues/?jql=..."');
    console.log('\nMake sure to set in .env:');
    console.log('  JIRA_EMAIL=your-email@example.com');
    console.log('  JIRA_API_TOKEN=your-api-token');
    console.log('\nGet your API token from: https://id.atlassian.com/manage-profile/security/api-tokens');
    process.exit(1);
  }

  const input = args.join(' ');
  const jql = extractJQL(input);
  
  console.log('\n🔍 JIRA Issue Search (Direct API)');
  console.log('=' .repeat(80));
  console.log('JQL Query:', jql);
  console.log('=' .repeat(80));

  try {
    let allIssues = [];
    let startAt = 0;
    const pageSize = 50;
    let total = 0;
    
    // Fetch first page
    console.log(`\nFetching issues...`);
    const firstPage = await searchIssues(jql, startAt, pageSize);
    
    total = firstPage.total || 0;
    allIssues = firstPage.issues || [];
    
    console.log(`\n📊 Total issues found: ${total}`);
    
    // Handle pagination
    if (total > pageSize) {
      const rl = readline.createInterface({
        input: process.stdin,
        output: process.stdout
      });
      
      const answer = await new Promise(resolve => {
        rl.question(`\nShowing first ${pageSize} issues. Fetch all ${total} issues? (y/n) `, (answer) => {
          rl.close();
          resolve(answer.toLowerCase());
        });
      });
      
      if (answer === 'y' || answer === 'yes') {
        // Fetch remaining pages
        startAt = pageSize;
        while (startAt < total) {
          process.stdout.write(`\rFetching issues ${startAt + 1} to ${Math.min(startAt + pageSize, total)}...`);
          const page = await searchIssues(jql, startAt, pageSize);
          if (page.issues) {
            allIssues = allIssues.concat(page.issues);
          }
          startAt += pageSize;
        }
        console.log(' Done!');
      }
    }
    
    // Display results
    console.log('\n📋 Issues:\n');
    allIssues.forEach((issue, index) => {
      console.log(formatIssue(issue, index));
      console.log();
    });
    
    console.log(`✅ Displayed ${allIssues.length} of ${total} total issues`);
    
    // Export option
    if (allIssues.length > 0) {
      const rl = readline.createInterface({
        input: process.stdin,
        output: process.stdout
      });
      
      const answer = await new Promise(resolve => {
        rl.question('\nExport to CSV? (y/n) ', (answer) => {
          rl.close();
          resolve(answer.toLowerCase());
        });
      });
      
      if (answer === 'y' || answer === 'yes') {
        const csv = exportToCSV(allIssues);
        const filename = `jira-export-${Date.now()}.csv`;
        await import('fs').then(fs => fs.promises.writeFile(filename, csv));
        console.log(`\n📁 Exported to ${filename}`);
      }
    }
    
  } catch (error) {
    console.error('\n❌ Error:', error.message);
    console.error('\nMake sure you have set JIRA_EMAIL and JIRA_API_TOKEN in your .env file');
    console.error('Get your API token from: https://id.atlassian.com/manage-profile/security/api-tokens');
    process.exit(1);
  }
}

// Export issues to CSV
function exportToCSV(issues) {
  const headers = ['Key', 'Summary', 'Status', 'Type', 'Priority', 'Created', 'Assignee', 'Reporter', 'Link'];
  const rows = [headers.join(',')];
  
  for (const issue of issues) {
    const row = [
      issue.key,
      `"${issue.fields.summary.replace(/"/g, '""')}"`,
      issue.fields.status.name,
      issue.fields.issuetype.name,
      issue.fields.priority?.name || 'None',
      new Date(issue.fields.created).toLocaleDateString(),
      issue.fields.assignee?.displayName || '',
      issue.fields.reporter?.displayName || '',
      `${JIRA_BASE_URL}/browse/${issue.key}`
    ];
    rows.push(row.join(','));
  }
  
  return rows.join('\n');
}

// Export functions for use in other scripts
export {
  jiraAPI,
  extractJQL,
  searchIssues,
  getIssue,
  formatIssue,
  exportToCSV
};

// Run main if executed directly
if (import.meta.url === `file://${process.argv[1]}`) {
  main().catch(console.error);
}