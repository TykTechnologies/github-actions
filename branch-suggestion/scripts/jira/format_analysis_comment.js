#!/usr/bin/env node

// Takes analysis JSON and outputs ADF-formatted JIRA comment
// Usage: node format_analysis_comment.js <analysis-json>

const analysisJson = process.argv[2];

if (!analysisJson) {
  console.error('Usage: node format_analysis_comment.js <analysis-json>');
  process.exit(1);
}

let analysis;
try {
  analysis = JSON.parse(analysisJson);
} catch (error) {
  console.error('Error parsing JSON:', error.message);
  process.exit(1);
}

const timestamp = new Date().toISOString();

// Determine panel type based on complexity
const panelType =
  analysis.complexity === 'High' || analysis.complexity === 'Large' ? 'error' :
  analysis.complexity === 'Medium' ? 'warning' :
  'success';

// Build ADF comment structure
const commentBody = {
  type: 'doc',
  version: 1,
  content: [
    {
      type: 'heading',
      attrs: { level: 3 },
      content: [
        { type: 'emoji', attrs: { shortName: ':robot:' } },
        { type: 'text', text: ' AI Complexity Analysis' }
      ]
    },
    {
      type: 'panel',
      attrs: { panelType },
      content: [
        {
          type: 'paragraph',
          content: [
            { type: 'text', text: 'Complexity: ', marks: [{ type: 'strong' }] },
            { type: 'text', text: analysis.complexity || 'Not specified' }
          ]
        },
        {
          type: 'paragraph',
          content: [
            { type: 'text', text: 'Priority: ', marks: [{ type: 'strong' }] },
            { type: 'text', text: analysis.priority || 'Not specified' }
          ]
        }
      ]
    }
  ]
};

// Add summary
if (analysis.summary) {
  commentBody.content.push({
    type: 'paragraph',
    content: [
      { type: 'text', text: 'Summary: ', marks: [{ type: 'strong' }] },
      { type: 'text', text: analysis.summary }
    ]
  });
}

// Add components
if (analysis.components && analysis.components.length > 0) {
  commentBody.content.push({
    type: 'paragraph',
    content: [
      { type: 'text', text: 'Components: ', marks: [{ type: 'strong' }] },
      { type: 'text', text: analysis.components.join(', ') }
    ]
  });
}

if (analysis.versions) {
  const versions = analysis.versions;
  const versionContent = [];

  if (versions.affected_versions && versions.affected_versions.length > 0) {
    versionContent.push({
      type: 'paragraph',
      content: [
        { type: 'text', text: 'Affected Versions: ', marks: [{ type: 'strong' }] },
        { type: 'text', text: versions.affected_versions.join(', ') }
      ]
    });
  }

  if (versions.last_working_version) {
    versionContent.push({
      type: 'paragraph',
      content: [
        { type: 'text', text: 'Last Working Version: ', marks: [{ type: 'strong' }] },
        { type: 'text', text: versions.last_working_version }
      ]
    });
  }

  if (versions.regression_introduced_in) {
    versionContent.push({
      type: 'paragraph',
      content: [
        { type: 'text', text: 'Regression Introduced In: ', marks: [{ type: 'strong' }] },
        { type: 'text', text: versions.regression_introduced_in }
      ]
    });
  }
}

// Add reproducible (for bugs)
if (analysis.reproducible !== undefined) {
  commentBody.content.push({
    type: 'paragraph',
    content: [
      { type: 'text', text: 'Reproducible: ', marks: [{ type: 'strong' }] },
      { type: 'text', text: analysis.reproducible ? 'Yes' : 'No' }
    ]
  });

  // Add version content if any exists
  if (versionContent.length > 0) {
    commentBody.content.push({
      type: 'panel',
      attrs: { panelType: 'info' },
      content: [
        {
          type: 'heading',
          attrs: { level: 5 },
          content: [
            { type: 'text', text: 'Version Information' }
          ]
        },
        ...versionContent
      ]
    });
  }
}

// Add risk factors
if (analysis.riskFactors && analysis.riskFactors.length > 0) {
  commentBody.content.push({
    type: 'paragraph',
    content: [
      { type: 'text', text: 'Risk Factors: ', marks: [{ type: 'strong' }] },
      { type: 'text', text: analysis.riskFactors.join(', ') }
    ]
  });
}

// Add impact assessment
if (analysis.impact) {
  const impact = analysis.impact;
  const parts = [];
  if (impact.users_affected) parts.push(`Users: ${impact.users_affected}`);
  if (impact.business_impact) parts.push(`Business: ${impact.business_impact}`);
  if (impact.security_issue) parts.push('Security issue');
  if (impact.customer_escalation) parts.push('Customer escalation');

  if (parts.length > 0) {
    commentBody.content.push({
      type: 'paragraph',
      content: [
        { type: 'text', text: 'Impact: ', marks: [{ type: 'strong' }] },
        { type: 'text', text: parts.join(', ') }
      ]
    });
  }
}

// Add full analysis in expandable section
if (analysis.reasoning) {
  // Create detailed analysis text
  let detailedText = `**Full Analysis**\n\n`;
  detailedText += `${analysis.reasoning}\n\n`;

  if (analysis.requirements) {
    detailedText += `**Requirements:**\n`;
    detailedText += `- Testing: ${analysis.requirements.testing_effort || 'Not specified'}\n`;
    detailedText += `- UX: ${analysis.requirements.ux_effort || 'Not specified'}\n`;
    detailedText += `- Research: ${analysis.requirements.research_needed || 'Not specified'}\n`;
    detailedText += `- POC: ${analysis.requirements.poc_required || 'Not specified'}\n\n`;
  }

  if (analysis.recommendations && analysis.recommendations.length > 0) {
    detailedText += `**Recommendations:**\n`;
    analysis.recommendations.forEach(rec => {
      detailedText += `- ${rec}\n`;
    });
  }

  commentBody.content.push(
    {
      type: 'rule'
    },
    {
      type: 'expand',
      attrs: { title: 'Full Analysis Report' },
      content: [
        {
          type: 'codeBlock',
          attrs: { language: 'markdown' },
          content: [
            { type: 'text', text: detailedText }
          ]
        }
      ]
    }
  );
}

// Add timestamp
commentBody.content.push({
  type: 'paragraph',
  content: [
    { type: 'text', text: `Last updated: ${timestamp}`, marks: [{ type: 'em' }] }
  ]
});

// Output ADF JSON
console.log(JSON.stringify(commentBody));
