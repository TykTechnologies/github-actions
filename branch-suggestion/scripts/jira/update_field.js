#!/usr/bin/env node
import dotenv from 'dotenv';
dotenv.config();

const JIRA_BASE_URL = 'https://tyktech.atlassian.net';
const JIRA_EMAIL = process.env.JIRA_EMAIL;
const JIRA_API_TOKEN = process.env.JIRA_API_TOKEN;
const THEME_FIELD_ID = process.env.JIRA_THEME_FIELD_ID || 'customfield_10750';

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

async function updateField(issueKey, fieldId, value) {
  // Try to parse value as JSON if it looks like JSON
  let parsedValue = value;
  if (typeof value === 'string' &&
      (value.startsWith('[') || value.startsWith('{'))) {
    try {
      parsedValue = JSON.parse(value);
    } catch (e) {
      console.log('Value appears to be JSON but could not be parsed, using as string');
    }
  }

  await jiraAPI(`/issue/${issueKey}`, {
    method: 'PUT',
    body: JSON.stringify({
      fields: {
        [fieldId]: parsedValue
      }
    })
  });

  // Format value for display
  const displayValue = typeof parsedValue === 'object' ?
      JSON.stringify(parsedValue) : parsedValue;


  console.log(`✅ Set ${fieldId} to '${displayValue}' on ${issueKey}`);
}

async function main() {
  const args = process.argv.slice(2);

  if (args.length < 2 || args.includes('--help')) {
    console.log('Usage: node jira-update-field.js <ticket-id> <field-id> <value>');
    console.log('\nExamples:');
    console.log('  node jira-update-field.js TT-12345 customfield_10750 Authentication');
    console.log('  node jira-update-field.js TT-12345 customfield_10750 "API Gateway"');
    console.log('\nTo find custom field IDs:');
    console.log('  1. Go to JIRA Settings → Issues → Custom fields');
    console.log('  2. Click on your field and check the URL');
    console.log('  3. Look for customfield_XXXXX in the URL');
    console.log('\nCommon fields:');
    console.log(`  AI Theme field: ${THEME_FIELD_ID}`);
    process.exit(0);
  }

  const ticketId = args[0];
  const fieldId = args[1];
  const value = args.slice(2).join(' ');

  if (!value) {
    console.error('❌ Missing value');
    process.exit(1);
  }

  try {
    await updateField(ticketId, fieldId, value);
  } catch (error) {
    console.error('❌ Error:', error.message);
    process.exit(1);
  }
}

main().catch(console.error);
