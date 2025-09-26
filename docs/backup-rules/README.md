# Backup Rules Directory

This directory contains backups of rule files before major modifications.

## File Index

### `00-ai-assistant-behavioral-constraints-pre-checkpoint-d-merge.mdc`
- **Date**: September 25, 2025
- **Purpose**: Backup before integrating CHECKPOINT D: Build Error Investigation
- **Changes Made**:
  - Merged standalone `07-build-error-investigation.mdc` into main behavioral constraints
  - Added CHECKPOINT D for comprehensive build error analysis
  - Removed duplicate content sections
  - Enhanced validation requirements and decision gates
- **Original Location**: `.cursor/rules/00-ai-assistant-behavioral-constraints.mdc`
- **Restore Command**:
  ```bash
  cp docs/backup-rules/00-ai-assistant-behavioral-constraints-pre-checkpoint-d-merge.mdc .cursor/rules/00-ai-assistant-behavioral-constraints.mdc
  ```

## Backup Naming Convention

Format: `[original-filename]-[description]-[action].mdc`

Examples:
- `pre-checkpoint-d-merge` - Before adding CHECKPOINT D
- `pre-aggregation` - Before consolidating multiple files
- `pre-refactor` - Before major structural changes

## Maintenance

- Keep backups for major rule changes
- Document the reason for each backup
- Include restore instructions
- Clean up old backups after 6 months unless historically significant
