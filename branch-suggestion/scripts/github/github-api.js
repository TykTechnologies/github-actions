#!/usr/bin/env node
import dotenv from 'dotenv';

dotenv.config();

// Configuration
const GITHUB_TOKEN = process.env.GITHUB_TOKEN;

/**
 * Generic GitHub API request wrapper
 * @param {string} endpoint - API endpoint (e.g., '/repos/owner/repo/branches')
 * @param {object} options - Fetch options (method, body, etc.)
 */

async function githubAPI(endpoint, options = {}) {
  if (!GITHUB_TOKEN) {
    throw new Error('GITHUB_TOKEN must be set in .env file');
  }

    const url = endpoint.startsWith('http')
        ? endpoint
        : `https://api.github.com${endpoint}`;
  const response = await fetch(url, {
    ...options,
    headers: {
      'Authorization': `token ${GITHUB_TOKEN}`,
      'Accept': 'application/vnd.github.v3+json',
      ...options.headers
    }
  });

  if (!response.ok) {
      const error = await response.text();
      throw new Error(`GitHub API Error (${response.status}): ${error}`);
  }

  // Handle 204 No Content responses
    if(response.status === 204 || response.headers.get('content-length') === '0'){
        return {}
    }

    return response.json();
}

/**
 * Get all branches for a repository
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @returns {Promise<Array>} Array of branch objects
 */
async function getRepoBranches(owner, repo) {
    const branches = await githubAPI(`/repos/${owner}/${repo}/branches`);
    return branches;
}

/**
 * Get a specific pull request
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @param {number} prNumber - PR number
 * @returns {Promise<object>} PR object
 */
async function getPullRequest(owner, repo, prNumber) {
    const pr = await githubAPI(`/repos/${owner}/${repo}/pulls/${prNumber}`);
    return pr;
}

/**
 * Create a comment on a pull request
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @param {number} prNumber - PR number
 * @param {string} body - Comment body
 * @returns {Promise<object>} Created comment object
 */
async function createPRComment(owner, repo, prNumber, body) {
    const comment = await githubAPI(`/repos/${owner}/${repo}/issues/${prNumber}/comments`, {
        method: 'POST',
        body: JSON.stringify({ body })
    });
    return comment;
}

/**
 * Update an existing comment
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @param {number} commentId - Comment ID
 * @param {string} body - New comment body
 * @returns {Promise<object>} Updated comment object
 */
async function updatePRComment(owner, repo, commentId, body) {
    const comment = await githubAPI(`/repos/${owner}/${repo}/issues/comments/${commentId}`, {
        method: 'PATCH',
        body: JSON.stringify({ body })
    });
    return comment;
}

/**
 * List all comments on a pull request
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @param {number} prNumber - PR number
 * @returns {Promise<Array>} Array of comment objects
 */
async function listPRComments(owner, repo, prNumber) {
    const comments = await githubAPI(`/repos/${owner}/${repo}/issues/${prNumber}/comments`);
    return comments;
}

/**
 * Find a comment by a marker/identifier in its body
 * Useful for updating existing bot comments instead of creating duplicates
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @param {number} prNumber - PR number
 * @param {string} marker - Unique marker to search for (e.g., '<!-- branch-suggestions -->')
 * @returns {Promise<object|null>} Comment object if found, null otherwise
 */
async function findCommentByMarker(owner, repo, prNumber, marker) {
    const comments = await listPRComments(owner, repo, prNumber);
    return comments.find(comment => comment.body.includes(marker)) || null;
}

export {
    githubAPI,
    getRepoBranches,
    getPullRequest,
    createPRComment,
    updatePRComment,
    listPRComments,
    findCommentByMarker
};