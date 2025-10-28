#!/usr/bin/env node
import dotenv from 'dotenv';
import { jiraAPI, getIssue } from './jira-api.js';

// Only load .env if JIRA_TOKEN is not already set (to avoid log output in CI)
// Silence dotenv v17+ logging
if (!process.env.JIRA_TOKEN) {
    process.env.DOTENV_LOG_LEVEL = 'error';
    dotenv.config();
}

/**
 * Extract JIRA ticket key from text (e.g., PR title, branch name)
 * Looks for patterns like TT-12345, TYK-456, etc.
 *
 * Examples:
 *   "TT-12345: Fix authentication bug" → "TT-12345"
 *   "feature/TT-12345-fix-auth" → "TT-12345"
 *   "Fix auth (TT-12345)" → "TT-12345"
 *
 * @param {string} text - Text to search (PR title, branch name, etc.)
 * @returns {string|null} First JIRA ticket key found, or null
 */
function extractJiraTicket(text) {
    if (!text) return null;

    // Match pattern: 2+ uppercase letters, dash, 1+ digits
    // Works in titles: "TT-12345: Fix bug"
    // Works in branches: "feature/TT-12345-fix-bug"
    const match = text.match(/\b([A-Z]{2,})-(\d+)\b/);
    return match ? match[0] : null;
}


/**
 * Parse a version string into semantic version components
 * Handles various formats: "5.8.1", "v5.8.1", "Tyk Gateway 5.8.1", "5.8", "5"
 * @param {string} versionString - Version string to parse
 * @returns {object|null} Object with {major, minor, patch, original} or null if invalid
 */
function parseVersion(versionString) {
    if (!versionString) return null;

    // Remove common prefixes: "v5.8.1" → "5.8.1", "Tyk 5.8.1" → "5.8.1", "TIB 1.7.0" → "1.7.0"
    const cleaned = versionString
        .replace(/^v/i, '')
        .replace(/^Tyk\s+/i, '')
        .replace(/^TIB\s+/i, '')
        .trim();

    // Match semantic version: X.Y.Z or X.Y or X
    const match = cleaned.match(/^(\d+)(?:\.(\d+))?(?:\.(\d+))?/);

    if (!match) return null;

    return {
        major: parseInt(match[1], 10),
        minor: match[2] ? parseInt(match[2], 10) : null,
        patch: match[3] ? parseInt(match[3], 10) : null,
        original: versionString
    };
}

/**
 * Get fix versions from a JIRA ticket
 * @param {string} ticketKey - JIRA ticket key (e.g., 'TT-12345')
 * @returns {Promise<object>} Object with ticket info and fix versions
 */
async function getFixVersions(ticketKey) {
    try {
        // Fetch ticket with all fields
        const ticket = await getIssue(ticketKey);

        const fixVersions = ticket.fields.fixVersions || [];

        return {
            ticket: ticketKey,
            summary: ticket.fields.summary,
            priority: ticket.fields.priority?.name || 'Unknown',
            issueType: ticket.fields.issuetype?.name || 'Unknown',
            fixVersions: fixVersions.map(v => ({
                name: v.name,
                id: v.id,
                released: v.released || false,
                parsed: parseVersion(v.name)
            }))
        };
    } catch (error) {
        throw new Error(`Failed to fetch JIRA ticket ${ticketKey}: ${error.message}`);
    }
}

async function main() {
    const args = process.argv.slice(2);


    if (args.length < 1) {
        console.log('Usage: node get-fixversion.js <ticket-key-or-text>');
        console.log('\nExamples:');
        console.log('  # Direct ticket key');
        console.log('  node get-fixversion.js TT-12345');
        console.log('');
        console.log('  # From PR title');
        console.log('  node get-fixversion.js "TT-12345: Fix authentication bug"');
        console.log('');
        console.log('  # From branch name');
        console.log('  node get-fixversion.js "feature/TT-12345-fix-auth"');
        console.log('\nOutput: JSON object with ticket info and fix versions');
        console.log('\nExit codes:');
        console.log('  0 - Success (fix versions found)');
        console.log('  1 - Error (no ticket found or no fix versions)');
        process.exit(1);
    }

    const input = args[0];

    // Try to extract ticket key if not already in correct format
    const ticketKey = input.match(/^[A-Z]{2,}-\d+$/) ? input : extractJiraTicket(input);

    if (!ticketKey) {
        console.error(JSON.stringify({
            error: 'No JIRA ticket found in input',
            input: input
        }));
        process.exit(1);
    }

    try {
        const result = await getFixVersions(ticketKey);

        // Check if no fix versions found (acceptance criteria: fail if missing)
        if (result.fixVersions.length === 0) {
            console.error(JSON.stringify({
                error: 'No fix versions found in JIRA ticket',
                ticket: ticketKey,
                summary: result.summary,
                priority: result.priority,
                issueType: result.issueType
            }));
            process.exit(1);
        }

        console.log(JSON.stringify(result, null, 2));

    } catch (error) {
        console.error(JSON.stringify({
            error: error.message,
            ticket: ticketKey
        }));
        process.exit(1);
    }

}

// Export functions for use in other scripts
export {
    extractJiraTicket,
    getFixVersions,
    parseVersion
};


// Run main if executed directly
if (import.meta.url === `file://${process.argv[1]}`) {
    main();
}

