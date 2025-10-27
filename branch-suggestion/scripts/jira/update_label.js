#!/usr/bin/env node
import dotenv from 'dotenv';
dotenv.config();

const JIRA_BASE_URL = 'https://tyktech.atlassian.net';
const JIRA_EMAIL = process.env.JIRA_EMAIL;
const JIRA_API_TOKEN = process.env.JIRA_API_TOKEN;

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

  if (response.status === 204) {
    return {};
  }

  return response.json();
}

async function getCurrentLabels(issueKey) {
  const issue = await jiraAPI(`/issue/${issueKey}?fields=labels`);
  return issue.fields?.labels || [];
}

async function updateLabels(issueKey, labels) {
  await jiraAPI(`/issue/${issueKey}`, {
    method: 'PUT',
    body: JSON.stringify({
      fields: { labels }
    })
  });
}

async function addLabel(issueKey, label) {
  const currentLabels = await getCurrentLabels(issueKey);

  if (currentLabels.includes(label)) {
    console.log(`Label '${label}' already exists on ${issueKey}`);
    return;
  }

  const newLabels = [...currentLabels, label];
  await updateLabels(issueKey, newLabels);
  console.log(`✅ Added label '${label}' to ${issueKey}`);
}

async function removeLabel(issueKey, label) {
  const currentLabels = await getCurrentLabels(issueKey);

  if (!currentLabels.includes(label)) {
    console.log(`Label '${label}' does not exist on ${issueKey}`);
    return;
  }

  const newLabels = currentLabels.filter(l => l !== label);
  await updateLabels(issueKey, newLabels);
  console.log(`✅ Removed label '${label}' from ${issueKey}`);
}

async function setLabel(issueKey, labelTemplate, value) {
  // Replace {value} placeholder in template
  const label = labelTemplate.replace('{value}', value);
  const currentLabels = await getCurrentLabels(issueKey);

  // Extract prefix from template (everything before {value})
  const prefixMatch = labelTemplate.match(/^(.+?)-?\{value\}/);
  if (prefixMatch) {
    const prefix = prefixMatch[1];
    // Remove any existing labels with the same prefix
    const filteredLabels = currentLabels.filter(l => !l.startsWith(`${prefix}-`));
    const newLabels = [...filteredLabels, label];
    await updateLabels(issueKey, newLabels);
  } else {
    // No template, just add the label
    if (currentLabels.includes(label)) {
      console.log(`Label '${label}' already exists on ${issueKey}`);
      return;
    }
    const newLabels = [...currentLabels, label];
    await updateLabels(issueKey, newLabels);
  }

  console.log(`✅ Set label '${label}' on ${issueKey}`);
}

async function main() {
  const args = process.argv.slice(2);

  if (args.length < 2 || args.includes('--help')) {
    console.log('Usage: node jira-update-labels.js <ticket-id> <action> <value>');
    console.log('\nActions:');
    console.log('  add <label>                    Add a label');
    console.log('  remove <label>                 Remove a label');
    console.log('  set <template> <value>         Set label using template with {value} placeholder');
    console.log('\nTemplate format:');
    console.log('  - Use {value} as placeholder: "AI-Complexity-{value}"');
    console.log('  - Auto-removes existing labels with same prefix');
    console.log('  - For bug/feature detection, use separate calls or wrapper script');
    console.log('\nExamples:');
    console.log('  node jira-update-labels.js TT-12345 add customer_bug');
    console.log('  node jira-update-labels.js TT-12345 remove customer_bug');
    console.log('  node jira-update-labels.js TT-12345 set "AI-Complexity-{value}" Medium');
    console.log('  node jira-update-labels.js TT-12345 set "AI-Priority-{value}" High');
    console.log('  node jira-update-labels.js TT-12345 set "AI-Theme-{value}" Authentication');
    process.exit(0);
  }

  const ticketId = args[0];
  const action = args[1];
  const value = args[2];
  const extraValue = args[3];

  try {
    if (action === 'add' && value) {
      await addLabel(ticketId, value);
    } else if (action === 'remove' && value) {
      await removeLabel(ticketId, value);
    } else if (action === 'set' && value && extraValue) {
      await setLabel(ticketId, value, extraValue);
    } else {
      console.error('❌ Invalid action or missing value');
      console.log('Run with --help for usage');
      process.exit(1);
    }
  } catch (error) {
    console.error('❌ Error:', error.message);
    process.exit(1);
  }
}

main().catch(console.error);
