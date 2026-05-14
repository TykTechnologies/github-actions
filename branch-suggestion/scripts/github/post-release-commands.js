#!/usr/bin/env node
import {
  createPRComment,
  parseRepo,
  findCommentByMarker,
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

  // Collect all unique branches with priority 'required' or 'recommended'
  const targetBranches = new Set();
  for (const result of matchResults) {
    for (const branchMatch of result.branches) {
      if (
        branchMatch.branch !== "master" &&
        (branchMatch.priority === "required" ||
          branchMatch.priority === "recommended")
      ) {
        targetBranches.add(branchMatch.branch);
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
    `Found ${targetBranches.size} target branches for release commands: ${Array.from(targetBranches).join(", ")}`,
  );

  for (const branch of targetBranches) {
    const marker = `<!-- auto-release: ${branch} -->`;
    const body = `/release to ${branch}\n\n${marker}`;

    try {
      // Check if this specific release command has already been posted
      console.log(`Checking for existing comment for ${branch}...`);
      const existingComment = await findCommentByMarker(
        owner,
        repo,
        prNumber,
        marker,
      );

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
