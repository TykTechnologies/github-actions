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
 * @param {string} text - Text to search
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
 * Detect which component repositories a version applies to based on prefix
 * @param {string} versionString - Version string to detect component from
 * @returns {Array<string>} Array of repository names this version applies to
 */
function detectComponent(versionString) {
    if (!versionString) return [];

    const normalized = versionString.trim();

    // Check for TIB (Tyk Identity Broker)
    if (/^TIB\s+/i.test(normalized)) {
        return ['tyk-identity-broker'];
    }

    // Check for Pump (Tyk Pump)
    if (/^(Tyk\s+)?Pump\s+/i.test(normalized)) {
        return ['tyk-pump'];
    }

    // Check for MDCB
    if (/^MDCB\s+/i.test(normalized)) {
        return ['tyk-sink'];
    }

    // Check for Tyk or Tyk Gateway (shared release cadence)
    if (/^Tyk(\s+Gateway)?\s+/i.test(normalized)) {
        return ['tyk', 'tyk-analytics', 'tyk-analytics-ui'];
    }

    // Unknown prefix - return empty array (no filtering)
    return [];
}

/**
 * Parse a version string into semantic version components
 * @param {string} versionString - Version string to parse
 * @returns {object|null} Object with {major, minor, patch, original, component} or null if invalid
 */
function parseVersion(versionString) {
    if (!versionString) return null;

    // Detect component before removing prefixes
    const component = detectComponent(versionString);

    // Remove common prefixes: "v5.8.1" → "5.8.1", "Tyk 5.8.1" → "5.8.1", "TIB 1.7.0" → "1.7.0"
    const cleaned = versionString
        .replace(/^v/i, '')
        .replace(/^Tyk(\s+Gateway)?\s+/i, '')
        .replace(/^TIB\s+/i, '')
        .replace(/^(Tyk\s+)?Pump\s+/i, '')
        .replace(/^MDCB\s+/i, '')
        .trim();

    // Match semantic version: X.Y.Z or X.Y or X
    const match = cleaned.match(/^(\d+)(?:\.(\d+))?(?:\.(\d+))?/);

    if (!match) return null;

    return {
        major: parseInt(match[1], 10),
        minor: match[2] ? parseInt(match[2], 10) : null,
        patch: match[3] ? parseInt(match[3], 10) : null,
        original: versionString,
        component: component
    };
}

/**
 * Get fix versions from a JIRA ticket
 * @param {string} ticketKey - JIRA ticket key
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
        console.log('Usage: node get-fixversion.js <pr-title> [<branch-name>]');
        console.log('\nExamples:');
        console.log('  # From PR title only');
        console.log('  node get-fixversion.js "TT-12345: Fix authentication bug"');
        console.log('');
        console.log('  # From PR title and branch name (branch name takes precedence)');
        console.log('  node get-fixversion.js "TT-12345: Fix bug" "feature/TT-67890-fix-auth"');
        console.log('');
        console.log('  # Direct ticket key');
        console.log('  node get-fixversion.js TT-12345');
        console.log('\nOutput: JSON object with ticket info and fix versions');
        console.log('\nExit codes:');
        console.log('  0 - Success (fix versions found)');
        console.log('  1 - Error (ticket found but no fix versions set)');
        console.log('  2 - No JIRA ticket found');
        process.exit(1);
    }

    const prTitle = args[0];
    const branchName = args[1]; // Optional

    let ticketKey = null;

    // Priority 1: Try to extract from branch name (if provided)
    if (branchName) {
        ticketKey = extractJiraTicket(branchName);
    }

    // Priority 2: Try to extract from PR title (if not found in branch name)
    if (!ticketKey && prTitle) {
        // Check if prTitle is already a valid ticket key format
        if (prTitle.match(/^[A-Z]{2,}-\d+$/)) {
            ticketKey = prTitle;
        } else {
            ticketKey = extractJiraTicket(prTitle);
        }
    }

    if (!ticketKey) {
        console.error(JSON.stringify({
            error: 'No JIRA ticket found in PR title or branch name',
            prTitle: prTitle,
            branchName: branchName || 'not provided'
        }));
        process.exit(2);
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
    parseVersion,
    detectComponent
};


// Run main if executed directly
if (import.meta.url === `file://${process.argv[1]}`) {
    main();
}

