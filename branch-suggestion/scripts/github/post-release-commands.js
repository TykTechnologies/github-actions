#!/usr/bin/env node
import {
  createPRComment,
  parseRepo,
  listPRComments,
  getPullRequest,
} from "./github-api.js";

/**
 * Post /release comments for each required/recommended branch
 * @param {string} repository - Repository in "owner/repo" format
 * @param {number} prNumber - Pull request number
 * @param {object} matchData - JSON output from match-branches.js
 */
async function postReleaseComments(repository, prNumber, matchData) {
  const { owner, repo } = parseRepo(repository);
  const { matchResults } = matchData;

  if (!matchResults || !Array.isArray(matchResults)) {
    console.log("No match results found. Skipping release comments.");
    return;
  }

  // Resolve base branch (e.g. master or main) dynamically
  let baseBranch = process.env.GITHUB_BASE_REF || "master";
  try {
    console.log(`Fetching PR details to resolve base branch...`);
    const pr = await getPullRequest(owner, repo, prNumber);
    baseBranch = pr.base.ref;
    console.log(`Resolved base branch: ${baseBranch}`);
  } catch (error) {
    console.warn(`⚠️ Failed to fetch PR details, using base branch fallback '${baseBranch}': ${error.message}`);
  }

  // Collect all unique branches with their highest priority ('required' > 'recommended')
  const targetBranches = new Map();
  for (const result of matchResults) {
    for (const branchMatch of (result.branches || [])) {
      if (
        branchMatch.branch !== baseBranch &&
        (branchMatch.priority === "required" ||
          branchMatch.priority === "recommended")
      ) {
        const existingPriority = targetBranches.get(branchMatch.branch);
        if (branchMatch.priority === "required" || !existingPriority) {
          targetBranches.set(branchMatch.branch, branchMatch.priority);
        }
      }
    }
  }

  if (targetBranches.size === 0) {
    console.log(
      "No eligible release branches found (required or recommended).",
    );
    return;
  }

  console.log(
    `Found ${targetBranches.size} target branches for release commands: ${Array.from(targetBranches.keys()).join(", ")}`,
  );

  console.log(`Fetching existing PR comments...`);
  const comments = await listPRComments(owner, repo, prNumber);
  const errors = [];

  for (const [branch, priority] of targetBranches.entries()) {
    // Security check: validate branch name format to prevent newline/command injection
    if (!/^[\w.\-\/]+$/.test(branch)) {
      throw new Error(`Security check failed: Invalid branch name format '${branch}'`);
    }

    const marker = `<!-- auto-release: ${branch} -->`;
    const body = `/release to ${branch}\n\n${marker}`;

    try {
      // Check if this specific release command has already been posted
      console.log(`Checking for existing comment for ${branch}...`);
      const existingComment = comments.find((comment) => comment.body.includes(marker)) || null;

      if (existingComment) {
        console.log(
          `ℹ️  Release comment for ${branch} already exists. Skipping.`,
        );
        continue;
      }

      console.log(
        `Posting comment: "/release to ${branch}" on PR #${prNumber}...`,
      );
      await createPRComment(owner, repo, prNumber, body);
      console.log(`✅ Successfully posted release comment for ${branch}`);
    } catch (error) {
      console.error(
        `❌ Failed to process release comment for ${branch}: ${error.message}`,
      );
      errors.push({ branch, priority, error });
    }
  }

  if (errors.length > 0) {
    const requiredFailures = errors.filter(e => e.priority === 'required');
    const errorMessages = errors.map(e => `  - ${e.branch} (${e.priority}): ${e.error.message}`).join('\n');
    
    if (requiredFailures.length > 0) {
      throw new Error(`Failed to post release comments for required branches:\n${errorMessages}`);
    } else {
      console.error(`⚠️ Some recommended release comments failed to post:\n${errorMessages}`);
    }
  }
}

// Main execution when run directly
async function main() {
  const args = process.argv.slice(2);

  if (args.length < 3) {
    console.log(
      "Usage: node post-release-commands.js <owner/repo> <pr-number> <match-data-json>",
    );
    process.exit(1);
  }

  try {
    const repository = args[0];
    const prNumber = parseInt(args[1], 10);
    const matchData = JSON.parse(args[2]);

    if (isNaN(prNumber) || prNumber < 1) {
      throw new Error("PR number must be a positive integer");
    }

    await postReleaseComments(repository, prNumber, matchData);
  } catch (error) {
    console.error(`❌ Error: ${error.message}`);
    process.exit(1);
  }
}

// Run main if executed directly
if (import.meta.url === `file://${process.argv[1]}`) {
  main();
}

export { postReleaseComments };
