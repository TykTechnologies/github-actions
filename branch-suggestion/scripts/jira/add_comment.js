#!/usr/bin/env node
import dotenv from 'dotenv';
import fs from 'fs';
dotenv.config();

// Simple markdown to ADF converter
function markdownToADF(markdown) {
  const lines = markdown.split('\n');
  const content = [];
  let inCodeBlock = false;
  let codeBlockLines = [];
  let codeBlockLang = '';
  let inExpandable = false;
  let expandableTitle = '';
  let expandableLines = [];

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];

    // Expandable section markers: <details> or <expand title="...">
    const expandStart = line.match(/^<(?:details|expand)(?:\s+title=["'](.+?)["'])?>/);
    if (expandStart) {
      inExpandable = true;
      expandableTitle = expandStart[1] || 'Details';
      expandableLines = [];
      continue;
    }

    const expandEnd = line.match(/^<\/(?:details|expand)>/);
    if (expandEnd && inExpandable) {
      // Create expand node with collected content
      const expandContent = markdownToADF(expandableLines.join('\n'));
      content.push({
        type: 'expand',
        attrs: { title: expandableTitle },
        content: expandContent
      });
      inExpandable = false;
      expandableTitle = '';
      expandableLines = [];
      continue;
    }

    if (inExpandable) {
      expandableLines.push(line);
      continue;
    }

    // Code blocks
    if (line.startsWith('```')) {
      if (inCodeBlock) {
        // End of code block
        content.push({
          type: 'codeBlock',
          attrs: { language: codeBlockLang },
          content: [{ type: 'text', text: codeBlockLines.join('\n') }]
        });
        codeBlockLines = [];
        codeBlockLang = '';
        inCodeBlock = false;
      } else {
        // Start of code block
        codeBlockLang = line.substring(3).trim() || 'text';
        inCodeBlock = true;
      }
      continue;
    }

    if (inCodeBlock) {
      codeBlockLines.push(line);
      continue;
    }

    // Headings
    if (line.startsWith('### ')) {
      content.push({
        type: 'heading',
        attrs: { level: 3 },
        content: [{ type: 'text', text: line.substring(4) }]
      });
      continue;
    }
    if (line.startsWith('## ')) {
      content.push({
        type: 'heading',
        attrs: { level: 2 },
        content: [{ type: 'text', text: line.substring(3) }]
      });
      continue;
    }
    if (line.startsWith('# ')) {
      content.push({
        type: 'heading',
        attrs: { level: 1 },
        content: [{ type: 'text', text: line.substring(2) }]
      });
      continue;
    }

    // Horizontal rule
    if (line.trim() === '---' || line.trim() === '***') {
      content.push({ type: 'rule' });
      continue;
    }

    // Empty line
    if (!line.trim()) {
      continue;
    }

    // List items
    if (line.match(/^[\s]*[-*]\s/)) {
      const indent = line.match(/^(\s*)/)[0].length;
      const text = line.replace(/^[\s]*[-*]\s/, '');

      // Parse inline formatting
      const paraContent = parseInlineFormatting(text);

      content.push({
        type: 'bulletList',
        content: [{
          type: 'listItem',
          content: [{
            type: 'paragraph',
            content: paraContent
          }]
        }]
      });
      continue;
    }

    // Regular paragraph
    const paraContent = parseInlineFormatting(line);
    content.push({
      type: 'paragraph',
      content: paraContent
    });
  }

  return content;
}

// Parse inline formatting (bold, italic, code)
function parseInlineFormatting(text) {
  const content = [];
  let currentText = '';
  let i = 0;

  while (i < text.length) {
    // Bold **text**
    if (text[i] === '*' && text[i + 1] === '*') {
      if (currentText) {
        content.push({ type: 'text', text: currentText });
        currentText = '';
      }
      i += 2;
      let boldText = '';
      while (i < text.length && !(text[i] === '*' && text[i + 1] === '*')) {
        boldText += text[i];
        i++;
      }
      i += 2;
      content.push({ type: 'text', text: boldText, marks: [{ type: 'strong' }] });
      continue;
    }

    // Italic *text*
    if (text[i] === '*') {
      if (currentText) {
        content.push({ type: 'text', text: currentText });
        currentText = '';
      }
      i++;
      let italicText = '';
      while (i < text.length && text[i] !== '*') {
        italicText += text[i];
        i++;
      }
      i++;
      content.push({ type: 'text', text: italicText, marks: [{ type: 'em' }] });
      continue;
    }

    // Inline code `text`
    if (text[i] === '`') {
      if (currentText) {
        content.push({ type: 'text', text: currentText });
        currentText = '';
      }
      i++;
      let codeText = '';
      while (i < text.length && text[i] !== '`') {
        codeText += text[i];
        i++;
      }
      i++;
      content.push({ type: 'text', text: codeText, marks: [{ type: 'code' }] });
      continue;
    }

    currentText += text[i];
    i++;
  }

  if (currentText) {
    content.push({ type: 'text', text: currentText });
  }

  return content.length > 0 ? content : [{ type: 'text', text: '' }];
}

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

// Create rich ADF formatted comment
function createCommentBody(text, identifier = null) {
  const timestamp = new Date().toISOString();

  // If text is already ADF JSON, parse and return it
  if (typeof text === 'string' && text.trim().startsWith('{')) {
    try {
      const parsed = JSON.parse(text);
      if (parsed.type === 'doc') {
        return parsed; // Already ADF format
      }
    } catch (e) {
      // Not JSON, continue with text formatting
    }
  }

  const content = [];

  // Add heading with robot emoji if identifier provided
  if (identifier) {
    content.push({
      type: 'heading',
      attrs: { level: 3 },
      content: [
        { type: 'emoji', attrs: { shortName: ':robot:' } },
        { type: 'text', text: ` ${identifier}` }
      ]
    });
  }

  // Check if text contains markdown formatting (headings, lists, code blocks)
  const hasMarkdown = /^#{1,3}\s|^[-*]\s|^```|^\*\*|^---/.test(text) || text.includes('\n');

  if (hasMarkdown) {
    // Parse markdown to ADF
    const markdownContent = markdownToADF(text);
    content.push(...markdownContent);
  } else {
    // Simple text in a panel
    content.push({
      type: 'panel',
      attrs: { panelType: 'info' },
      content: [
        {
          type: 'paragraph',
          content: [
            { type: 'text', text: String(text) }
          ]
        }
      ]
    });
  }

  // Add timestamp
  content.push({
    type: 'paragraph',
    content: [
      { type: 'text', text: `Last updated: ${timestamp}`, marks: [{ type: 'em' }] }
    ]
  });

  return {
    type: 'doc',
    version: 1,
    content
  };
}

async function findExistingComment(issueKey, identifier) {
  const commentsData = await jiraAPI(`/issue/${issueKey}/comment`);

  for (const comment of commentsData.comments || []) {
    const bodyText = JSON.stringify(comment.body);

    if (bodyText.includes(identifier)) {
      return comment.id;
    }
  }

  return null;
}

async function addComment(issueKey, text, identifier = null) {
  const commentBody = createCommentBody(text, identifier);

  // If identifier is provided, check for existing comment
  if (identifier) {
    const existingCommentId = await findExistingComment(issueKey, identifier);

    if (existingCommentId) {
      // Delete old comment
      await jiraAPI(`/issue/${issueKey}/comment/${existingCommentId}`, {
        method: 'DELETE'
      });
      console.log(`🗑️  Deleted existing comment with identifier '${identifier}' (ID: ${existingCommentId})`);
    }
  }

  // Add new comment
  await jiraAPI(`/issue/${issueKey}/comment`, {
    method: 'POST',
    body: JSON.stringify({ body: commentBody })
  });

  if (identifier) {
    console.log(`✅ Added comment with identifier '${identifier}' to ${issueKey}`);
  } else {
    console.log(`✅ Added comment to ${issueKey}`);
  }
}

async function main() {
  const args = process.argv.slice(2);

  if (args.length === 0 || args.includes('--help')) {
    console.log('Usage: node jira-add-comment.js <ticket-id> <text> [--comment-id <id>] [--file <path>]');
    console.log('\nOptions:');
    console.log('  --comment-id <id>   Unique identifier for this comment. If a comment with');
    console.log('                      this ID exists, it will be replaced (deleted + recreated)');
    console.log('  --file <path>       Read comment text/ADF JSON from file');
    console.log('\nFormat:');
    console.log('  - Plain text: Automatically formatted as ADF');
    console.log('  - ADF JSON: Auto-detected and used directly (for rich formatting)');
    console.log('\nExamples:');
    console.log('  # Simple comment');
    console.log('  node jira-add-comment.js TT-12345 "This is a test comment"');
    console.log('');
    console.log('  # Comment with identifier (replaces existing if found)');
    console.log('  node jira-add-comment.js TT-12345 "Analysis text" --comment-id AI-Analysis');
    console.log('');
    console.log('  # Read from file (text or ADF JSON)');
    console.log('  node jira-add-comment.js TT-12345 --file analysis.txt');
    console.log('  node jira-add-comment.js TT-12345 --file comment.json --comment-id AI-Analysis');
    process.exit(0);
  }

  const ticketId = args[0];

  // Check for --comment-id flag
  const commentIdIndex = args.indexOf('--comment-id');
  let identifier = null;
  if (commentIdIndex !== -1) {
    identifier = args[commentIdIndex + 1];
    if (!identifier) {
      console.error('❌ Missing identifier after --comment-id');
      process.exit(1);
    }
  }

  // Check if --file flag is present
  const fileIndex = args.indexOf('--file');
  let text;

  if (fileIndex !== -1) {
    // --file flag present
    const filePath = args[fileIndex + 1];
    if (!filePath) {
      console.error('❌ Missing file path after --file');
      process.exit(1);
    }
    if (!fs.existsSync(filePath)) {
      console.error(`❌ File not found: ${filePath}`);
      process.exit(1);
    }
    text = fs.readFileSync(filePath, 'utf8');
  } else {
    // No --file flag, text is the second argument
    text = args[1];
    if (!text) {
      console.error('❌ Missing comment text');
      process.exit(1);
    }
  }

  try {
    await addComment(ticketId, text, identifier);
  } catch (error) {
    console.error('❌ Error:', error.message);
    process.exit(1);
  }
}

main().catch(console.error);
