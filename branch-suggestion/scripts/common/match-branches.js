#!/usr/bin/env node
import dotenv from 'dotenv';

// Only load .env if JIRA_TOKEN is not already set (to avoid log output in CI)
// Silence dotenv v17+ logging
if (!process.env.JIRA_TOKEN) {
    process.env.DOTENV_LOG_LEVEL = 'error';
    dotenv.config();
}

/**
 * Generate candidate branch names based on a parsed version
 * According to the branching strategy:
 * - For 5.8.1: try release-5.8, release-5, master
 * - For 5.8.0: try release-5.8, release-5, master
 * - For 5.0.0: try release-5, master
 *
 * @param {object} parsedVersion - {major, minor, patch, original}
 * @returns {Array} Array of branch name candidates in priority order
 */

function generateBranchCandidates(parsedVersion) {
    if (!parsedVersion) return ['master'];

    const { major, minor, patch } = parsedVersion;
    const candidates = [];

    // For patch releases (X.Y.Z where Z > 0): need release-X.Y
    if (patch !== null && patch > 0 && minor !== null) {
        candidates.push(`release-${major}.${minor}`);
    }

    // For minor releases (X.Y.0 or X.Y): need release-X.Y
    if (minor !== null) {
        candidates.push(`release-${major}.${minor}`);
    }

    // For any version: might need release-X (major version branch)
    if (major !== null) {
        candidates.push(`release-${major}`);
    }

    // Always include master as fallback
    candidates.push('master');

    // Remove duplicates while preserving order
    return [...new Set(candidates)];
}

/**
 * Match fix versions to actual branches in the repository
 *
 * @param {Array} fixVersions - Array of fix version objects with parsed field
 * @param {Array} repoBranches - Array of branch objects from GitHub API
 * @returns {Array} Array of match results with branch suggestions
 */

function matchBranches(fixVersions, repoBranches) {
    const branchNames = repoBranches.map(b => b.name);
    const results = [];

    for (const fixVersion of fixVersions) {
        const candidates = generateBranchCandidates(fixVersion.parsed);
        const matches = [];

        // Check which candidate branches actually exist
        for (const candidate of candidates) {
            if (branchNames.includes(candidate)) {
                matches.push({
                    branch: candidate,
                    reason: getBranchReason(candidate, fixVersion),
                    priority: getBranchPriority(candidate, fixVersion)
                });
            }
        }

        // If no release branches found, only suggest master
        if (matches.length === 0 || (matches.length === 1 && matches[0].branch === 'master')) {
            results.push({
                fixVersion: fixVersion.name,
                parsed: fixVersion.parsed,
                branches: [{
                    branch: 'master',
                    reason: 'No matching release branches found. Fix will be included in future releases.',
                    priority: 'required'
                }],
                warning: 'Expected release branches not found in repository'
            });
        } else {
            results.push({
                fixVersion: fixVersion.name,
                parsed: fixVersion.parsed,
                branches: matches
            });
        }
    }

    return results;
}

/**
 * Get human-readable reason for why a branch is suggested
 * @param {string} branch - Branch name
 * @param {object} fixVersion - Fix version object
 * @returns {string} Explanation
 */
function getBranchReason(branch, fixVersion) {
    const { major, minor, patch } = fixVersion.parsed || {};

    if (branch === 'master') {
        return 'Main development branch - ensures fix is in all future releases';
    }

    if (branch === `release-${major}.${minor}`) {
        if (patch > 0) {
            return `Minor version branch for ${major}.${minor}.x patches - required for creating ${fixVersion.name}`;
        } else {
            return `Minor version branch for ${major}.${minor}.x releases`;
        }
    }

    if (branch === `release-${major}`) {
        return `Major version branch for all ${major}.x releases`;
    }

    return `Release branch for version ${fixVersion.name}`;
}

/**
 * Determine priority level for a branch suggestion
 * @param {string} branch - Branch name
 * @param {object} fixVersion - Fix version object
 * @returns {string} 'required' | 'recommended' | 'optional'
 */
function getBranchPriority(branch, fixVersion) {
    const { major, minor, patch } = fixVersion.parsed || {};

    // Master is always required
    if (branch === 'master') {
        return 'required';
    }

    // For patch releases (5.8.1), release-5.8 is required
    if (patch > 0 && branch === `release-${major}.${minor}`) {
        return 'required';
    }

    // Minor and major release branches are recommended
    if (branch === `release-${major}.${minor}` || branch === `release-${major}`) {
        return 'recommended';
    }

    return 'optional';
}

/**
 * Format branch suggestions as markdown for PR comment
 * @param {Array} matchResults - Results from matchBranches()
 * @param {object} jiraTicket - JIRA ticket info
 * @returns {string} Markdown formatted comment
 */
function formatBranchSuggestions(matchResults, jiraTicket = {}) {
    const lines = [];

    lines.push('## 🎯 Recommended Merge Targets');
    lines.push('');

    if (jiraTicket.ticket) {
        lines.push(`Based on JIRA ticket **${jiraTicket.ticket}**${jiraTicket.summary ? `: ${jiraTicket.summary}` : ''}`);
        lines.push('');
    }

    for (const result of matchResults) {
        lines.push(`### Fix Version: ${result.fixVersion}`);
        lines.push('');

        if (result.warning) {
            lines.push(`> ⚠️ **Warning:** ${result.warning}`);
            lines.push('');
        }

        const required = result.branches.filter(b => b.priority === 'required');
        const recommended = result.branches.filter(b => b.priority === 'recommended');
        const optional = result.branches.filter(b => b.priority === 'optional');

        if (required.length > 0) {
            lines.push('**Required:**');
            for (const branch of required) {
                lines.push(`- \`${branch.branch}\` - ${branch.reason}`);
            }
            lines.push('');
        }

        if (recommended.length > 0) {
            lines.push('**Recommended:**');
            for (const branch of recommended) {
                lines.push(`- \`${branch.branch}\` - ${branch.reason}`);
            }
            lines.push('');
        }

        if (optional.length > 0) {
            lines.push('**Optional:**');
            for (const branch of optional) {
                lines.push(`- \`${branch.branch}\` - ${branch.reason}`);
            }
            lines.push('');
        }
    }

    lines.push('---');
    lines.push('');
    lines.push('### 📋 Workflow');
    lines.push('1. Merge this PR to `master` first');
    lines.push('2. Use the `/release` bot to backport to release branches');
    lines.push('3. Example: `/release to release-5.8`');
    lines.push('');
    lines.push('<!-- branch-suggestions -->');

    return lines.join('\n');
}

// Main execution when run directly
async function main() {
    const args = process.argv.slice(2);

    if (args.length < 2) {
        console.log('Usage: node match-branches.js <fix-versions-json> <repo-branches-json>');
        console.log('\nExample:');
        console.log('  node match-branches.js \'[{"name":"5.8.1","parsed":{"major":5,"minor":8,"patch":1}}]\' \'[{"name":"master"},{"name":"release-5.8"}]\'');
        console.log('\nOr pipe from other commands:');
        console.log('  VERSIONS=$(node scripts/jira/get-fixversion.js TT-12345)');
        console.log('  BRANCHES=$(gh api repos/TykTechnologies/tyk/branches | jq -c \'[.[] | {name: .name}]\')');
        console.log('  node match-branches.js "$VERSIONS" "$BRANCHES"');
        process.exit(1);
    }

    try {
        const fixVersionsInput = JSON.parse(args[0]);
        const repoBranches = JSON.parse(args[1]);

        // Handle if fixVersionsInput is the full JIRA response
        const fixVersions = fixVersionsInput.fixVersions || fixVersionsInput;
        const jiraTicket = fixVersionsInput.ticket ? {
            ticket: fixVersionsInput.ticket,
            summary: fixVersionsInput.summary
        } : {};

        const matchResults = matchBranches(fixVersions, repoBranches);

        // Output JSON for pipeline processing
        console.log(JSON.stringify({
            jiraTicket,
            matchResults,
            markdown: formatBranchSuggestions(matchResults, jiraTicket)
        }, null, 2));

    } catch (error) {
        console.error(JSON.stringify({
            error: error.message,
            details: 'Failed to parse input JSON or match branches'
        }));
        process.exit(1);
    }
}

// Export functions for use in other scripts
export {
    generateBranchCandidates,
    matchBranches,
    getBranchReason,
    getBranchPriority,
    formatBranchSuggestions
};

// Run main if executed directly
if (import.meta.url === `file://${process.argv[1]}`) {
    main();
}

