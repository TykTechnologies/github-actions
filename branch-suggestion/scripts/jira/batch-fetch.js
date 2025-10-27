#!/usr/bin/env node
import { execSync } from 'child_process';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

// Suppress dotenv output in quiet mode
if (process.argv.includes('--quiet')) {
  process.env.DOTENV_SILENT = 'true';
  // Redirect console.log temporarily during dotenv load
  const originalLog = console.log;
  console.log = () => {};
  const dotenv = await import('dotenv');
  dotenv.config();
  console.log = originalLog;
} else {
  const dotenv = await import('dotenv');
  dotenv.config();
}

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// JIRA API configuration
const JIRA_EMAIL = process.env.JIRA_EMAIL;
const JIRA_API_TOKEN = process.env.JIRA_API_TOKEN;
const JIRA_BASE_URL = 'https://tyktech.atlassian.net';

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

async function searchIssues(jql, startAt = 0, maxResults = 50, pageToken = null) {
  const params = new URLSearchParams({
    jql: jql,
    maxResults: maxResults.toString(),
    fields: 'key,summary,status,labels,created,updated,issuetype,priority,assignee,reporter'
  });

  // Use pageToken if provided, otherwise use startAt
  if (pageToken) {
    params.append('nextPageToken', pageToken);
  } else {
    params.append('startAt', startAt.toString());
  }

  return jiraAPI(`/search/jql?${params}`, { method: 'GET' });
}

function extractJQLFromUrl(input) {
  try {
    const url = new URL(input);
    const jql = url.searchParams.get('jql');
    if (jql) {
      return decodeURIComponent(jql);
    }
  } catch (e) {
    // Not a URL, treat as JQL
  }
  return input;
}

async function fetchTicketData(jiraKey, forceRefresh = false) {
  try {
    // Always run unified-fetch to ensure we get Zendesk tickets
    // unified-fetch will use cache if available, so it's fast
    execSync(`node ${path.join(__dirname, '../common/unified-fetch.js')} ${jiraKey}`, {
      stdio: ['pipe', 'pipe', 'pipe'],  // Capture all output
      cwd: __dirname
    });

    // Read cached JIRA data
    const jiraPath = path.join(__dirname, '../../.cache', 'jira', jiraKey);
    const jiraPathRelative = path.join('.cache', 'jira', jiraKey);
    const ticketJsonPath = path.join(jiraPath, 'ticket.json');

    if (fs.existsSync(ticketJsonPath)) {
      const ticketData = JSON.parse(fs.readFileSync(ticketJsonPath, 'utf8'));

      // Get all Zendesk tickets that unified-fetch found
      // Since unified-fetch just ran, we need to check what it cached
      const zendeskIds = await findAllZendeskTicketsForJira(jiraKey, ticketData);

      // Count JIRA attachments
      let attachmentCount = 0;
      if (fs.existsSync(jiraPath)) {
        const files = fs.readdirSync(jiraPath);
        attachmentCount = files.filter(f => f.startsWith('attachment_')).length;
      }

      // Fetch Zendesk ticket details
      const zendeskTickets = [];
      for (const zendeskId of zendeskIds) {
        const zendeskPath = path.join(__dirname, '../../.cache', 'zendesk', String(zendeskId));
        const zendeskPathRelative = path.join('.cache', 'zendesk', String(zendeskId));
        const zendeskJsonPath = path.join(zendeskPath, 'ticket.json');

        if (fs.existsSync(zendeskJsonPath)) {
          const zendeskData = JSON.parse(fs.readFileSync(zendeskJsonPath, 'utf8'));

          // Count Zendesk attachments
          let zendeskAttachments = 0;
          if (fs.existsSync(zendeskPath)) {
            const files = fs.readdirSync(zendeskPath);
            zendeskAttachments = files.filter(f => f.startsWith('attachment_')).length;
          }

          zendeskTickets.push({
            id: zendeskId,
            subject: zendeskData.ticket?.subject || '',
            status: zendeskData.ticket?.status || '',
            priority: zendeskData.ticket?.priority || '',
            created_at: zendeskData.ticket?.created_at || '',
            updated_at: zendeskData.ticket?.updated_at || '',
            requester: zendeskData.ticket?.requester?.name || '',
            organization: zendeskData.ticket?.organization?.name || '',
            attachments: zendeskAttachments,
            cachePath: zendeskPathRelative
          });
        }
      }

      return {
        key: jiraKey,
        fetchSuccess: true,
        summary: ticketData.fields?.summary || '',
        status: ticketData.fields?.status?.name || '',
        issueType: ticketData.fields?.issuetype?.name || '',
        priority: ticketData.fields?.priority?.name || '',
        labels: ticketData.fields?.labels || [],
        created: ticketData.fields?.created || '',
        updated: ticketData.fields?.updated || '',
        assignee: ticketData.fields?.assignee?.displayName || null,
        reporter: ticketData.fields?.reporter?.displayName || '',
        attachments: attachmentCount,
        zendeskTickets: zendeskTickets,
        cachePath: jiraPathRelative
      };
    }

    return {
      key: jiraKey,
      fetchSuccess: false,
      error: 'No cached data found after fetch'
    };

  } catch (error) {
    return {
      key: jiraKey,
      fetchSuccess: false,
      error: error.message
    };
  }
}

async function findAllZendeskTicketsForJira(jiraKey, jiraData) {
  const zendeskIds = new Set();

  // Method 1: Look in JIRA content for mentioned tickets
  const content = JSON.stringify(jiraData);
  const zendeskPattern = /zendesk\.com\/agent\/tickets\/(\d+)/g;
  const matches = content.matchAll(zendeskPattern);
  for (const match of matches) {
    zendeskIds.add(match[1]);
  }

  // Method 2: Run zendesk-api.js to search for tickets
  // This is what unified-fetch does to find related tickets
  try {
    const output = execSync(`node ${path.join(__dirname, '../zendesk/zendesk-api.js')} ${jiraKey}`, {
      cwd: __dirname,
      encoding: 'utf8',
      stdio: ['pipe', 'pipe', 'pipe']
    });

    // Parse ticket IDs from output
    const ticketPattern = /Ticket #(\d+)/g;
    const ticketMatches = output.matchAll(ticketPattern);
    for (const match of ticketMatches) {
      zendeskIds.add(match[1]);
    }
  } catch (error) {
    // If zendesk-api.js fails, continue with what we have
  }

  return [...zendeskIds];
}

async function main() {
  const args = process.argv.slice(2);

  if (args.length === 0 || args.includes('--help')) {
    console.log('Usage: node batch-fetch.js <jql-query|url> [options]');
    console.log('\nOptions:');
    console.log('  --limit <n>      Limit number of tickets to fetch (default: all)');
    console.log('  --single         Fetch only the first ticket');
    console.log('  --pretty         Pretty print JSON output');
    console.log('  --quiet          Suppress progress messages (only output JSON)');
    console.log('  --force          Force refresh even if data is cached');
    console.log('\nExamples:');
    console.log('  node batch-fetch.js "project = TT AND status = Open"');
    console.log('  node batch-fetch.js "project = TT AND key = TT-14937" --single');
    console.log('  node batch-fetch.js "labels = customer_bug" --limit 5 --pretty');
    console.log('  node batch-fetch.js "key = TT-102" --force  # Force refresh from APIs');
    console.log('\nOutput:');
    console.log('  JSON array of fetched tickets with metadata');
    console.log('\nNote:');
    console.log('  By default, uses cached data if available. Use --force to refresh.');
    process.exit(0);
  }

  const jqlInput = args[0];
  const singleMode = args.includes('--single');
  const prettyPrint = args.includes('--pretty');
  const quietMode = args.includes('--quiet');
  const forceRefresh = args.includes('--force');

  // Get limit if specified
  let limit = singleMode ? 1 : null;
  const limitIndex = args.indexOf('--limit');
  if (limitIndex !== -1 && args[limitIndex + 1]) {
    limit = parseInt(args[limitIndex + 1]);
  }

  if (!quietMode) {
    console.error('\n🔍 JIRA Batch Fetcher');
    console.error('=' .repeat(60));
  }

  try {
    // Extract JQL from URL if needed
    const jql = extractJQLFromUrl(jqlInput);

    if (!quietMode) {
      console.error(`\n📝 JQL Query: ${jql}`);
      if (limit) {
        console.error(`📊 Limit: ${limit} ticket(s)`);
      }
    }

    // Search for issues
    if (!quietMode) {
      console.error('\n🔎 Searching for tickets...');
    }

    const firstPage = await searchIssues(jql, 0, limit || 50);

    let allIssues = firstPage.issues || [];

    // Apply limit if specified
    if (limit && allIssues.length > limit) {
      allIssues = allIssues.slice(0, limit);
    }

    if (!quietMode) {
      console.error(`✅ Found ${allIssues.length} ticket(s)`);
    }

    if (allIssues.length === 0) {
      console.log(JSON.stringify([], null, prettyPrint ? 2 : 0));
      return;
    }

    // Fetch data for each ticket
    const results = [];

    for (let i = 0; i < allIssues.length; i++) {
      const issue = allIssues[i];

      if (!quietMode) {
        const jiraPath = path.join(__dirname, '../../.cache', 'jira', issue.key);
        const isCached = fs.existsSync(path.join(jiraPath, 'ticket.json'));

        if (isCached && !forceRefresh) {
          console.error(`\n[${i + 1}/${allIssues.length}] Loading ${issue.key} from cache...`);
        } else {
          console.error(`\n[${i + 1}/${allIssues.length}] Fetching ${issue.key}...`);
        }
      }

      const fetchResult = await fetchTicketData(issue.key, forceRefresh);

      // Merge JIRA search result with fetch result
      const ticketInfo = {
        ...fetchResult,
        // Add any additional fields from the search that aren't in fetch result
        searchData: {
          labels: issue.fields.labels || [],
          status: issue.fields.status?.name || '',
          summary: issue.fields.summary || ''
        }
      };

      results.push(ticketInfo);

      if (!quietMode) {
        if (fetchResult.fetchSuccess) {
          console.error(`  ✅ Fetched successfully`);
          if (fetchResult.attachments > 0) {
            console.error(`  📎 ${fetchResult.attachments} JIRA attachment(s)`);
          }
          if (fetchResult.zendeskTickets && fetchResult.zendeskTickets.length > 0) {
            console.error(`  🎫 ${fetchResult.zendeskTickets.length} Zendesk ticket(s) fetched`);
            for (const zTicket of fetchResult.zendeskTickets) {
              console.error(`     - #${zTicket.id}: ${zTicket.subject} (${zTicket.status})`);
              if (zTicket.attachments > 0) {
                console.error(`       📎 ${zTicket.attachments} attachment(s)`);
              }
            }
          }
        } else {
          console.error(`  ❌ Failed: ${fetchResult.error}`);
        }
      }
    }

    // Output results as JSON
    const output = {
      query: jql,
      timestamp: new Date().toISOString(),
      totalTickets: results.length,
      tickets: results
    };

    console.log(JSON.stringify(output, null, prettyPrint ? 2 : 0));

    if (!quietMode) {
      console.error('\n✅ Done');
    }

  } catch (error) {
    const errorOutput = {
      error: true,
      message: error.message,
      timestamp: new Date().toISOString()
    };

    console.log(JSON.stringify(errorOutput, null, prettyPrint ? 2 : 0));

    if (!quietMode) {
      console.error('\n❌ Error:', error.message);
    }

    process.exit(1);
  }
}

main().catch(error => {
  console.error('Fatal error:', error);
  process.exit(1);
});