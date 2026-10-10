# Spec Delta: ipfs-ui-components

## Purpose

Provides IPFS-specific UI components that render contract tool outputs in React 19 and are published as an A2UI v0.9 catalog.

## ADDED Requirements

### Requirement: Contract-bound components
Each component SHALL accept data shaped by a contract output schema. It SHALL render without network access beyond the data it is given.

#### Scenario: Render pin list offline
- **WHEN** the pin list component receives a valid `pin_ls` output with networking disabled
- **THEN** it renders every pin, and no network request is made

### Requirement: A2UI catalog
The library SHALL publish a generated A2UI v0.9 catalog manifest. Every catalog entry SHALL refer to an existing component.

#### Scenario: Catalog validation
- **WHEN** the catalog validation runs
- **THEN** every entry resolves to a component, and the manifest validates against the A2UI v0.9 catalog format

### Requirement: Accessibility and theming
Components MUST meet WCAG 2.2 AA in both light and dark themes.

#### Scenario: Automated accessibility check
- **WHEN** the axe and keyboard checks run on every component in both themes
- **THEN** no AA violation is reported

### Requirement: Approval card truthfulness
The approval card SHALL show approve and deny controls only when the pending confirmation is answerable in the UI; otherwise it SHALL state where approval happens.

#### Scenario: Out-of-band approval
- **WHEN** the card renders a pending confirmation that the native prompt resolves
- **THEN** no approve control is rendered and the text names the system prompt
