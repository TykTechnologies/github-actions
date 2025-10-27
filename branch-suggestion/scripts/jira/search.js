#!/usr/bin/env node
import { spawn } from 'child_process';
import readline from 'readline';
import dotenv from 'dotenv';
import { URL } from 'url';

dotenv.config();

const mcpJiraUrl = process.env.MCP_JIRA_URL;
const mcpJiraSecret = process.env.MCP_JIRA_SECRET;

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

async function searchJira(jql, startAt = 0, maxResults = 50) {
  return new Promise((resolve, reject) => {
    console.log(`\nFetching issues ${startAt + 1} to ${startAt + maxResults}...`);
    
    const mcpProcess = spawn('npx', [
      'mcp-remote',
      mcpJiraUrl,
      '--header',
      `Authorization: Bearer ${mcpJiraSecret}`
    ], {
      stdio: ['pipe', 'pipe', 'pipe']
    });

    let responses = [];
    let initialized = false;
    
    const rl = readline.createInterface({
      input: mcpProcess.stdout,
      output: process.stdout,
      terminal: false
    });

    // Send initialize request
    setTimeout(() => {
      const initRequest = {
        jsonrpc: '2.0',
        method: 'initialize',
        params: {
          protocolVersion: '2024-11-05',
          capabilities: {},
          clientInfo: {
            name: 'jira-search-client',
            version: '1.0.0'
          }
        },
        id: 1
      };
      mcpProcess.stdin.write(JSON.stringify(initRequest) + '\n');
    }, 500);

    // Send search request after initialization
    setTimeout(() => {
      const searchRequest = {
        jsonrpc: '2.0',
        method: 'tools/call',
        params: {
          name: 'searchForIssuesUsingJql',
          arguments: {
            jql: jql,
            startAt: startAt,
            maxResults: maxResults,
            fields: 'summary,status,issuetype,priority,created,assignee,reporter,customfield_10116'
          }
        },
        id: 2
      };
      mcpProcess.stdin.write(JSON.stringify(searchRequest) + '\n');
    }, 1000);

    rl.on('line', (line) => {
      try {
        const data = JSON.parse(line);
        responses.push(data);
        
        // Check if this is the search result
        if (data.id === 2) {
          mcpProcess.kill();
          if (data.result && data.result.content && data.result.content[0]) {
            try {
              const content = data.result.content[0].text;
              const searchResult = JSON.parse(content);
              resolve(searchResult);
            } catch (e) {
              // API error response
              reject(new Error(data.result.content[0].text));
            }
          } else if (data.error) {
            reject(new Error(data.error.message));
          }
        }
      } catch (e) {
        // Ignore non-JSON output
      }
    });

    mcpProcess.stderr.on('data', (data) => {
      // Suppress verbose stderr output
    });

    mcpProcess.on('close', () => {
      // Process closed
    });

    // Timeout after 10 seconds
    setTimeout(() => {
      mcpProcess.kill();
      reject(new Error('Search timeout'));
    }, 10000);
  });
}

async function main() {
  const args = process.argv.slice(2);
  
  if (args.length === 0) {
    console.log('Usage: node jira-search.js "<JQL query or JIRA URL>"');
    console.log('\nExamples:');
    console.log('  node jira-search.js "project = TT AND status != closed"');
    console.log('  node jira-search.js "https://tyktech.atlassian.net/jira/software/c/projects/TT/issues/?jql=..."');
    process.exit(1);
  }

  const input = args.join(' ');
  const jql = extractJQL(input);
  
  console.log('\n🔍 JIRA Issue Search');
  console.log('=' .repeat(80));
  console.log('JQL Query:', jql);
  console.log('=' .repeat(80));

  try {
    let allIssues = [];
    let startAt = 0;
    const pageSize = 50;
    let total = 0;
    
    // Fetch first page to get total count
    const firstPage = await searchJira(jql, startAt, pageSize);
    
    if (firstPage.errorMessages) {
      console.error('\n❌ Error:', firstPage.errorMessages.join(', '));
      process.exit(1);
    }
    
    total = firstPage.total || 0;
    allIssues = firstPage.issues || [];
    
    console.log(`\n📊 Total issues found: ${total}`);
    
    // Ask about pagination if more than one page
    if (total > pageSize) {
      console.log(`\nShowing first ${pageSize} issues. Fetch all ${total} issues? (y/n)`);
      
      const rl = readline.createInterface({
        input: process.stdin,
        output: process.stdout
      });
      
      const answer = await new Promise(resolve => {
        rl.question('', (answer) => {
          rl.close();
          resolve(answer.toLowerCase());
        });
      });
      
      if (answer === 'y' || answer === 'yes') {
        // Fetch remaining pages
        startAt = pageSize;
        while (startAt < total) {
          const page = await searchJira(jql, startAt, pageSize);
          if (page.issues) {
            allIssues = allIssues.concat(page.issues);
          }
          startAt += pageSize;
        }
      }
    }
    
    // Display results
    console.log('\n📋 Issues:\n');
    allIssues.forEach((issue, index) => {
      console.log(`${index + 1}. [${issue.key}] ${issue.fields.summary}`);
      console.log(`   Status: ${issue.fields.status.name}`);
      console.log(`   Type: ${issue.fields.issuetype.name}`);
      console.log(`   Priority: ${issue.fields.priority?.name || 'None'}`);
      console.log(`   Created: ${new Date(issue.fields.created).toLocaleDateString()}`);
      
      if (issue.fields.assignee) {
        console.log(`   Assignee: ${issue.fields.assignee.displayName}`);
      }
      if (issue.fields.reporter) {
        console.log(`   Reporter: ${issue.fields.reporter.displayName}`);
      }
      
      // Check for Customers Impacted field (customfield_10116 is common for this)
      if (issue.fields.customfield_10116) {
        console.log(`   Customers Impacted: Yes`);
      }
      
      console.log(`   Link: https://tyktech.atlassian.net/browse/${issue.key}`);
      console.log();
    });
    
    console.log(`\n✅ Displayed ${allIssues.length} of ${total} total issues`);
    
  } catch (error) {
    console.error('\n❌ Error:', error.message);
    process.exit(1);
  }
}

main().catch(console.error);