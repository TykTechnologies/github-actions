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
 * @param {object} parsedVersion - {major, minor, patch, original}
 * @returns {Array} Array of branch name candidates in priority order
 */

function generateBranchCandidates(parsedVersion) {
    if (!parsedVersion) return ['master'];

    const { major, minor, patch } = parsedVersion;
    const candidates = [];

    // First priority: exact match for the full version (X.Y.Z)
    if (patch !== null && minor !== null) {
        candidates.push(`release-${major}.${minor}.${patch}`);
    }

    // Second priority: minor version branch (X.Y) for patch releases
    if (patch !== null && patch > 0 && minor !== null) {
        candidates.push(`release-${major}.${minor}`);
    }

    // Third priority: minor version branch for minor releases (X.Y.0 or X.Y)
    if (minor !== null) {
        candidates.push(`release-${major}.${minor}`);
    }

    // Fourth priority: major version branch (X)
    if (major !== null) {
        candidates.push(`release-${major}`);
    }

    // Always include master as fallback
    candidates.push('master');

    // Remove duplicates while preserving order
    return [...new Set(candidates)];
}

/**
 * Filter fix versions by component to only include those relevant to the current repository
 * @param {Array} fixVersions - Array of fix version objects with parsed.component field
 * @param {string} repository - Repository in "owner/repo" format
 * @returns {Array} Filtered array of fix versions relevant to this repository
 */
function filterFixVersionsByRepository(fixVersions, repository) {
    if (!repository) {
        // No repository specified - return all fix versions (backward compatibility)
        return fixVersions;
    }

    // Extract repo name from "owner/repo" format
    const repoName = repository.split('/').pop();

    return fixVersions.filter(fixVersion => {
        const component = fixVersion.parsed?.component;

        // If no component array (empty), include it (applies to all repos)
        if (!component || component.length === 0) {
            return true;
        }

        // Check if current repo is in the component's applicable repos
        return component.includes(repoName);
    });
}

/**
 * Match fix versions to actual branches in the repository
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

    // Exact patch version match (e.g., release-5.10.1)
    if (patch !== null && branch === `release-${major}.${minor}.${patch}`) {
        return `Exact version branch for ${fixVersion.name} - specific patch release`;
    }

    // Minor version branch (e.g., release-5.10)
    if (branch === `release-${major}.${minor}`) {
        if (patch > 0) {
            return `Minor version branch for ${major}.${minor}.x patches - required for creating ${fixVersion.name}`;
        } else {
            return `Minor version branch for ${major}.${minor}.x releases`;
        }
    }

    // Major version branch (e.g., release-5)
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

    // Exact patch version match is required (e.g., release-5.10.1 for version 5.10.1)
    if (patch !== null && branch === `release-${major}.${minor}.${patch}`) {
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
    lines.push('');
    lines.push('1. **Merge this PR to `master` first**');
    lines.push('');

    // Collect all non-master branches that need cherry-picking using Set for O(1) operations
    const releaseBranchesSet = new Set();
    for (const result of matchResults) {
        for (const branch of result.branches) {
            if (branch.branch !== 'master') {
                releaseBranchesSet.add(branch.branch);
            }
        }
    }
    const releaseBranches = Array.from(releaseBranchesSet);

    if (releaseBranches.length > 0) {
        lines.push('2. **Cherry-pick to release branches** by commenting on the **merged PR**:');
        lines.push('');
        for (const branch of releaseBranches) {
            lines.push(`   - \`/release to ${branch}\``);
        }
        lines.push('');
        lines.push('3. **Automated backport** - The bot will automatically create backport PRs to the specified release branches');
    }

    lines.push('');
    lines.push('<!-- branch-suggestions -->');

    return lines.join('\n');
}

// Main execution when run directly
async function main() {
    const args = process.argv.slice(2);

    if (args.length < 2) {
        console.log('Usage: node match-branches.js <fix-versions-json> <repo-branches-json> [<repository>]');
        console.log('\nExample:');
        console.log('  node match-branches.js \'[{"name":"5.8.1","parsed":{"major":5,"minor":8,"patch":1}}]\' \'[{"name":"master"},{"name":"release-5.8"}]\' \'TykTechnologies/tyk\'');
        console.log('\nOr pipe from other commands:');
        console.log('  VERSIONS=$(node scripts/jira/get-fixversion.js TT-12345)');
        console.log('  BRANCHES=$(gh api repos/TykTechnologies/tyk/branches | jq -c \'[.[] | {name: .name}]\')');
        console.log('  node match-branches.js "$VERSIONS" "$BRANCHES" "TykTechnologies/tyk"');
        console.log('\nThe repository parameter is optional. If provided, filters fix versions to only show those relevant to the specified repository.');
        process.exit(1);
    }

    try {
        const fixVersionsInput = JSON.parse(args[0]);
        const repoBranches = JSON.parse(args[1]);
        const repository = args[2]; // Optional: filter by repository

        // Handle if fixVersionsInput is the full JIRA response
        let fixVersions = fixVersionsInput.fixVersions || fixVersionsInput;
        const jiraTicket = fixVersionsInput.ticket ? {
            ticket: fixVersionsInput.ticket,
            summary: fixVersionsInput.summary
        } : {};

        // Filter fix versions by repository if specified
        if (repository) {
            fixVersions = filterFixVersionsByRepository(fixVersions, repository);
        }

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
    formatBranchSuggestions,
    filterFixVersionsByRepository
};

// Run main if executed directly
if (import.meta.url === `file://${process.argv[1]}`) {
    main();
}

