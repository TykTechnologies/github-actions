#!/usr/bin/env node
import dotenv from 'dotenv';
import {
    createPRComment,
    findCommentByMarker
} from './github-api.js';

// Silence dotenv v17+ logging
process.env.DOTENV_LOG_LEVEL = 'error';
dotenv.config();

/**
 * Build the unique HTML marker for a given branch so we never post the same
 * release command twice (e.g., on a workflow re-run after merge).
 * @param {string} branch
 * @returns {string}
 */
function markerForBranch(branch) {
    return `<!-- auto-release: ${branch} -->`;
}

/**
 * Collect all non-master branches that are required or recommended across
 * all fix-version match results, deduplicated and in order.
 * @param {Array} matchResults - Output of match-branches.js
 * @returns {Array<string>} Branch names
 */
function collectReleaseBranches(matchResults) {
    const seen = new Set();
    const branches = [];

    for (const result of matchResults) {
        for (const b of result.branches) {
            if (b.branch === 'master' || b.branch === 'main') continue;
            if (b.priority === 'optional') continue;
            if (seen.has(b.branch)) continue;
            seen.add(b.branch);
            branches.push(b.branch);
        }
    }

    return branches;
}

/**
 * Post one `/release to <branch>` comment per release branch on the merged PR.
 * Skips branches that already have an auto-release comment (idempotent).
 * @param {string} owner
 * @param {string} repo
 * @param {number} prNumber
 * @param {Array} matchResults - Output of match-branches.js
 */
async function postReleaseCommands(owner, repo, prNumber, matchResults) {
    const branches = collectReleaseBranches(matchResults);

    if (branches.length === 0) {
        console.log('No release branches to post commands for.');
        return;
    }

    console.log(`Posting release commands for ${branches.length} branch(es): ${branches.join(', ')}`);

    for (const branch of branches) {
        const marker = markerForBranch(branch);

        // Idempotency check — skip if already posted
        const existing = await findCommentByMarker(owner, repo, prNumber, marker);
        if (existing) {
            console.log(`⏭️  Skipping ${branch} — command already posted (comment #${existing.id})`);
            continue;
        }

        const body = `/release to ${branch}\n\n${marker}`;
        const created = await createPRComment(owner, repo, prNumber, body);
        console.log(`✅ Posted /release to ${branch} (comment #${created.id})`);
    }
}

// Main execution when run directly
async function main() {
    const args = process.argv.slice(2);

    if (args.length < 3) {
        console.log('Usage: node post-release-commands.js <owner/repo> <pr-number> <match-results-json>');
        console.log('\nOr read match results from a file:');
        console.log('  node post-release-commands.js <owner/repo> <pr-number> --file <path>');
        process.exit(1);
    }

    const [repoArg, prArg] = args;
    const parts = repoArg.split('/');
    if (parts.length !== 2) {
        console.error('❌ Repository must be in format "owner/repo"');
        process.exit(1);
    }

    const [owner, repo] = parts;
    const prNumber = parseInt(prArg, 10);

    if (isNaN(prNumber) || prNumber < 1) {
        console.error('❌ PR number must be a positive integer');
        process.exit(1);
    }

    let matchResults;

    try {
        let raw;
        if (args[2] === '--file' && args[3]) {
            const fs = await import('fs');
            raw = fs.readFileSync(args[3], 'utf-8');
        } else {
            raw = args[2];
        }

        const parsed = JSON.parse(raw);
        // Accept either the full pipeline output { matchResults: [...] } or a bare array
        matchResults = parsed.matchResults || parsed;
    } catch (err) {
        console.error(`❌ Failed to parse match results: ${err.message}`);
        process.exit(1);
    }

    try {
        await postReleaseCommands(owner, repo, prNumber, matchResults);
    } catch (err) {
        console.error(`❌ Error posting release commands: ${err.message}`);
        process.exit(1);
    }
}

export { postReleaseCommands, collectReleaseBranches, markerForBranch };

if (import.meta.url === `file://${process.argv[1]}`) {
    main();
}
