# Spec Delta: webui-agent-chat

## Purpose

N5: mount the chat island in the GQAdonis/ipfs-webui fork.

## ADDED Requirements

### Requirement: Off by default and not loaded
The chat SHALL be disabled by default, and while disabled the island bundle SHALL NOT be requested.

#### Scenario: Default install
- **WHEN** the fork Web UI loads with default settings
- **THEN** no request for the island bundle is made and no chat launcher is shown
