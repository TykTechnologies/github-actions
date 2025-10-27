#!/usr/bin/env node
import dotenv from 'dotenv';
import { execSync } from 'child_process';
import path from 'path';
import { fileURLToPath } from 'url';

dotenv.config();

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const JIRA_BASE_URL = 'https://tyktech.atlassian.net';
const JIRA_EMAIL = process.env.JIRA_EMAIL;
const JIRA_API_TOKEN = process.env.JIRA_API_TOKEN;
const AFFECTED_VERSIONS_FIELD_ID = process.env.JIRA_AFFECTED_VERSIONS_FIELD_ID || 'customfield_10256';

async function main() {
    const args = process.argv.slice(2);

    if (args.length < 2) {
        console.log('Usage: node update_versions.js <ticket-id> <versions-json>');
        console.log('\nExample:');
        console.log('node update_versions.js TT-12345 \'{"affected_versions":["Tyk Portal 1.14.1","Tyk Gateway 5.0.0"],"last_working_version":"Tyk Portal 1.10.0"}\'');
        process.exit(1);
    }

    const ticketId = args[0];
    let versionsData;

    try {
        versionsData = JSON.parse(args[1]);
    } catch (error) {
        console.error('Error parsing versions JSON:', error.message);
        console.error('Provided JSON:', args[1]);
        process.exit(1);
    }

    try {
        await processAffectedVersions(ticketId, versionsData);
    } catch (error) {
        console.error('❌ Error:', error.message);
        process.exit(1);
    }
}

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

    // Add this check for 204 No Content responses
    if (response.status === 204 || response.headers.get('content-length') === '0') {
        return {}; // Return empty object for empty responses
    }

    return response.json();
}

async function getProjectVersions(projectKey) {
    console.log(`Getting available versions for project ${projectKey}...`);
    const versions = await jiraAPI(`/project/${projectKey}/versions`);
    console.log(`Found ${versions.length} available versions in project ${projectKey}`);
    return versions;
}

async function getTicketComponents(ticketId) {
    try {
        const ticket = await jiraAPI(`/issue/${ticketId}?fields=components`);
        return ticket.fields.components.map(c => c.name);
    } catch (error) {
        console.log(`⚠️ Could not fetch components for ${ticketId}: ${error.message}`);
        return [];
    }
}

async function processAffectedVersions(ticketId, versionsData) {
    if (!versionsData || !versionsData.affected_versions || versionsData.affected_versions.length === 0) {
        console.log(`No affected versions to update for ${ticketId}`);
        return;
    }

    try {
        // First, get available versions for the project
        const projectKey = ticketId.split('-')[0];

        // Get available versions
        const availableVersions = await getProjectVersions(projectKey);

        // Get ticket components for better version matching
        const components = await getTicketComponents(ticketId);
        const primaryComponent = components.length > 0 ? components[0] : '';
        console.log(`Ticket components: ${components.join(', ')}`);

        // Match affected versions with available versions in JIRA
        const versionIds = [];
        for (const affectedVersion of versionsData.affected_versions) {
            // Extract version number (remove any 'v' prefix)
            const versionNumber = affectedVersion.replace(/^[vV]/, '');

            // Try different formats of the version
            const versionVariants = [
                affectedVersion,                                // e.g., "Tyk Portal 1.14.1" or "v1.14.1"
                versionNumber,                                  // e.g., "1.14.1"
                `v${versionNumber}`,                            // e.g., "v1.14.1"
            ];

            // Add product-specific variants based on components or the version itself
            if (affectedVersion.includes('Portal') || primaryComponent.includes('Portal')) {
                versionVariants.push(`Tyk Portal ${versionNumber}`);
                versionVariants.push(`Portal ${versionNumber}`);
            }
            if (affectedVersion.includes('Gateway') || primaryComponent.includes('Gateway')) {
                versionVariants.push(`Tyk Gateway ${versionNumber}`);
                versionVariants.push(`Gateway ${versionNumber}`);
            }
            if (affectedVersion.includes('Dashboard') || primaryComponent.includes('Dashboard')) {
                versionVariants.push(`Tyk Dashboard ${versionNumber}`);
                versionVariants.push(`Dashboard ${versionNumber}`);
            }

            let matched = false;
            for (const variant of versionVariants) {
                const matchedVersion = availableVersions.find(v =>
                    v.name === variant ||
                    v.name.toLowerCase() === variant.toLowerCase()
                );

                if (matchedVersion) {
                    versionIds.push({ id: matchedVersion.id });
                    console.log(`Matched version: ${affectedVersion} → ${matchedVersion.name} (ID: ${matchedVersion.id})`);
                    matched = true;
                    break;
                }
            }

            // If no exact match, try partial matching
            if (!matched) {
                // Look for versions that contain the version number
                const partialMatches = availableVersions.filter(v =>
                    v.name.includes(versionNumber)
                );

                if (partialMatches.length === 1) {
                    // If only one match, use it
                    versionIds.push({ id: partialMatches[0].id });
                    console.log(`Partial match: ${affectedVersion} → ${partialMatches[0].name} (ID: ${partialMatches[0].id})`);
                    matched = true;
                } else if (partialMatches.length > 1) {
                    // If multiple matches, log them but don't use any
                    console.log(`⚠️ Multiple partial matches found for ${affectedVersion}:`);
                    partialMatches.forEach(v => console.log(`  - ${v.name} (ID: ${v.id})`));
                }
            }

            if (!matched) {
                console.log(`⚠️ No matching version found for: ${affectedVersion}`);
                // Log available versions for debugging
                console.log('Available versions:');
                availableVersions.slice(0, 10).forEach(v => console.log(`  - ${v.name} (ID: ${v.id})`));
                if (availableVersions.length > 10) {
                    console.log(`  ... and ${availableVersions.length - 10} more`);
                }
            }
        }

        if (versionIds.length === 0) {
            console.log(`No matching versions found in JIRA for ${ticketId}`);
            return;
        }

        // Instead of updating directly, use update_field.js
        console.log(`Updating ${ticketId} with ${versionIds.length} affected version(s)...`);

        // Convert version IDs array to JSON string
        const versionIdsJson = JSON.stringify(versionIds);

        // Call update_field.js to update the field
        try {
            console.log(`Calling update_field.js to set ${AFFECTED_VERSIONS_FIELD_ID} to ${versionIdsJson}`);

            // Use execSync to call update_field.js
            const updateFieldPath = path.join(__dirname, 'update_field.js');
            console.log(`update_field.js path: ${updateFieldPath}`);

            const command = `node "${updateFieldPath}" "${ticketId}" "${AFFECTED_VERSIONS_FIELD_ID}" '${versionIdsJson}'`;
            console.log(`Executing command: ${command}`);

            execSync(command, { stdio: 'inherit' });

            console.log(`✅ Successfully updated affected versions for ${ticketId}`);
        } catch (error) {
            console.error(`❌ Error calling update_field.js:`, error.message);
            throw new Error(`Failed to update field: ${error.message}`);
        }
    } catch (error) {
        console.error(`❌ Error processing versions for ${ticketId}:`, error.message);
        throw error;
    }
}

main().catch(console.error);