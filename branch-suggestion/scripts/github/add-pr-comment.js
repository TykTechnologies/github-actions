#!/usr/bin/env node
import dotenv from 'dotenv';
import fs from 'fs';
import {
    getPullRequest,
    createPRComment,
    updatePRComment,
    findCommentByMarker
} from './github-api.js';

// Silence dotenv v17+ logging
process.env.DOTENV_LOG_LEVEL = 'error';
dotenv.config();

const COMMENT_MARKER = '<!-- branch-suggestions -->';

/**
 * Add or update a comment on a GitHub PR
 * If a comment with the same marker exists, it will be updated instead of creating a new one
 *
 * @param {string} owner - Repository owner (e.g., 'TykTechnologies')
 * @param {string} repo - Repository name (e.g., 'tyk')
 * @param {number} prNumber - Pull request number
 * @param {string} body - Comment body (markdown)
 * @param {string} marker - Unique marker to identify the comment (default: COMMENT_MARKER)
 * @returns {Promise<object>} Created or updated comment
 */
async function addOrUpdateComment(owner, repo, prNumber, body, marker = COMMENT_MARKER) {
    // Ensure the marker is included in the comment body
    const commentBody = body.includes(marker) ? body : `${body}\n\n${marker}`;

    try {
        // Check if a comment with this marker already exists
        const existingComment = await findCommentByMarker(owner, repo, prNumber, marker);

        if (existingComment) {
            console.log(`Found existing comment (ID: ${existingComment.id}). Updating...`);
            const updated = await updatePRComment(owner, repo, existingComment.id, commentBody);
            console.log(`✅ Updated comment on PR #${prNumber}`);
            return updated;
        } else {
            console.log(`No existing comment found. Creating new comment...`);
            const created = await createPRComment(owner, repo, prNumber, commentBody);
            console.log(`✅ Created comment on PR #${prNumber}`);
            return created;
        }
    } catch (error) {
        throw new Error(`Failed to add/update comment on PR #${prNumber}: ${error.message}`);
    }
}

/**
 * Parse repository string in format "owner/repo"
 * @param {string} repoString - Repository in "owner/repo" format
 * @returns {object} {owner, repo}
 */
function parseRepo(repoString) {
    const parts = repoString.split('/');
    if (parts.length !== 2) {
        throw new Error('Repository must be in format "owner/repo"');
    }
    return { owner: parts[0], repo: parts[1] };
}

// Main execution when run directly
async function main() {
    const args = process.argv.slice(2);

    if (args.length < 2) {
        console.log('Usage: node add-pr-comment.js <owner/repo> <pr-number> [<comment-body>] [options]');
        console.log('\nOptions:');
        console.log('  --file <path>       Read comment body from file');
        console.log('  --marker <marker>   Custom marker (default: <!-- branch-suggestions -->)');
        console.log('\nExamples:');
        console.log('  # Direct comment text');
        console.log('  node add-pr-comment.js TykTechnologies/tyk 123 "## Suggested branches\\n- release-5.8"');
        console.log('');
        console.log('  # From file');
        console.log('  node add-pr-comment.js TykTechnologies/tyk 123 --file /tmp/comment.md');
        console.log('');
        console.log('  # Custom marker');
        console.log('  node add-pr-comment.js TykTechnologies/tyk 123 "Comment" --marker "<!-- custom -->"');
        process.exit(1);
    }

    try {
        const { owner, repo } = parseRepo(args[0]);
        const prNumber = parseInt(args[1], 10);

        if (isNaN(prNumber) || prNumber < 1) {
            throw new Error('PR number must be a positive integer');
        }

        let commentBody = args[2] || '';
        let marker = COMMENT_MARKER;

        // Parse options starting from position 2 (to handle --file in that position)
        for (let i = 2; i < args.length; i++) {
            if (args[i] === '--file' && args[i + 1]) {
                const filePath = args[i + 1];
                if (!fs.existsSync(filePath)) {
                    throw new Error(`File not found: ${filePath}`);
                }
                commentBody = fs.readFileSync(filePath, 'utf-8');
                i++; // Skip next arg
            } else if (args[i] === '--marker' && args[i + 1]) {
                marker = args[i + 1];
                i++; // Skip next arg
            }
        }

        if (!commentBody || commentBody.trim() === '') {
            throw new Error('Comment body cannot be empty. Use --file <path> to read from file.');
        }

        console.log(`Repository: ${owner}/${repo}`);
        console.log(`PR Number: ${prNumber}`);
        console.log(`Marker: ${marker}`);
        console.log('');

        const result = await addOrUpdateComment(owner, repo, prNumber, commentBody, marker);

        console.log('');
        console.log('Comment URL:', result.html_url);

    } catch (error) {
        console.error(`❌ Error: ${error.message}`);
        process.exit(1);
    }
}

// Export functions for use in other scripts
export {
    addOrUpdateComment,
    parseRepo,
    COMMENT_MARKER
};

// Run main if executed directly
if (import.meta.url === `file://${process.argv[1]}`) {
    main();
}