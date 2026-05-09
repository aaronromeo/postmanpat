# RulesGen Web Server Implementation Plan

## Overview

Convert the Python `postmanpat-generate-rules.py` script to a Go web server that provides an interactive UI for generating email filtering rules. The server runs analysis on-demand, presents clusters for review, and generates YAML rule files.

## Architecture

### Environment Variables

```bash
# Required
POSTMANPAT_RULESGEN_STORE=/path/to/rules-decisions.db
POSTMANPAT_RULESGEN_WATCH_OUT=/path/to/watch.yml
POSTMANPAT_RULESGEN_CLEANUP_OUT=/path/to/cleanup.yml
POSTMANPAT_RULESGEN_ONETIME_OUT=/path/to/cleanup-onetime.yml
POSTMANPAT_RULES_CONFIG=/path/to/imap-config.yaml

# Optional
POSTMANPAT_RULESGEN_PORT=8080  # Default: 8080
```

### CLI Command

```bash
# Start the rules generation server
postmanpat rulesgen serve [--port 8080]
```

## Current Status

**Step 1 Complete ✅** - Basic HTTP server with Hello World webpage
- Server starts and serves styled HTML at `GET /`
- CLI command `postmanpat rulesgen serve [--port 8080]` implemented
- Templates embedded using `//go:embed`
- Tests passing for default and custom port scenarios

**Step 2 Complete ✅** - Analysis integration with JSON API
- Analysis runs on-demand via `/api/analysis` endpoint
- Environment variables loaded for configuration
- Analysis logic implemented in `rulesgen/analyzer.go`
- Tests for config loading and analyze options

**Step 3 Complete ✅** - Store Analysis in Decisions Data Type  
- SQLite storage with comprehensive tests (`store_test.go` - 12 test cases)
- Mock store for unit testing
- `NewDecisionFromCluster()` helper function with validation
- Fixed "database is closed" bug in server startup

**Step 4 Complete ✅** - Render Analysis Output
- Mobile-responsive HTML template with touch optimization
- Cluster table with lens badges, counts, and examples
- Row selection with keyboard shortcuts
- Filters out decided clusters from display

**Step 5 Complete ✅** - Interactive Rule Creation with YAML Preview
- YAML preview generation functions (`yaml_preview.go`)
- API endpoint `/api/yaml-preview` for real-time YAML generation
- Client-side YAML syntax highlighting and responsive design
- Copy/download functionality for generated YAML
- Comprehensive YAML generation tests (125 tests passing)
- Subject pattern extraction and regex escaping helpers

## Implementation Steps

This implementation follows an iterative approach, building functionality step by step:

### Step 1: Hello World Webpage ✅
- [x] Create basic HTTP server with a simple "Hello World" response
- [x] Set up the Cobra command `rulesgen serve`
- [x] Verify server starts and responds on configured port
- [x] Create basic HTML template structure

**Deliverables:**
- `cli/rulesgen.go` - Cobra command with `--port` flag and `POSTMANPAT_RULESGEN_PORT` env var support
- `rulesgen/server.go` - HTTP server with embedded templates using `//go:embed`
- `rulesgen/templates/index.html` - Styled HTML template
- `cli/rulesgen_test.go` - Tests for default port (8080) and custom port scenarios

### Step 2: Run Analysis on Page Load ✅
- [x] Integrate analysis logic into server startup
- [x] Load configuration from environment variables
- [x] Run analysis when `GET /` is requested
- [x] Return analysis results as JSON (for debugging)

**Deliverables:**
- `rulesgen/analyzer.go` - Analysis logic with `Analyzer` struct and `Run()` method
- `rulesgen/server.go` - Updated to load config, create analyzer, and expose `/api/analysis` endpoint
- `cli/rulesgen.go` - Updated to load config from env vars and pass to server
- `rulesgen/analyzer_test.go` - Tests for config loading and analyze options

**Environment Variables Required:**
- `POSTMANPAT_RULES_CONFIG` - Path to IMAP config YAML
- `POSTMANPAT_RULESGEN_STORE` - Path to SQLite database
- `POSTMANPAT_RULESGEN_WATCH_OUT` - Path to watch.yml output
- `POSTMANPAT_RULESGEN_CLEANUP_OUT` - Path to cleanup.yml output  
- `POSTMANPAT_RULESGEN_ONETIME_OUT` - Path to cleanup-onetime.yml output

**API Endpoints:**
- `GET /` - Main UI page
- `GET /api/analysis` - Returns JSON analysis report

### Step 3: Store Analysis in Decisions Data Type ✅

**Part A: Core Implementation (Complete)**
- [x] Define core data structures (`types.go`)
- [x] Create SQLite storage for analysis results (`store.go`)
- [x] Add methods: CreateDecision, GetDecisions, HasDecision, GetAllDecisions
- [x] Map analysis clusters to decision data type
- [x] Integrate store with server

**Part B: Testing Implementation (Complete) ✅**
- [x] **Comprehensive SQLite Tests** (`store_test.go`)
  - `CreateDecision` - basic insert, ON CONFLICT update, duplicate prevention
  - `GetDecisions` - retrieve by lens/clusterID, empty results, ordering
  - `HasDecision` - true/false cases, non-existent clusters
  - `GetAllDecisions` - multiple entries, ordering by created_at
  - `GetAllDecidedClusterIDs` - map construction, key formatting (lens:cluster_id)
  - `DeleteDecision` - removal, idempotent delete
  - `CreateDecisionTx` - transaction success and rollback scenarios
  - Schema initialization verification
  
- [x] **Mapping Helper Function** (`types.go`)
  - Create `NewDecisionFromCluster(cluster analysis.Cluster, decisionType, action, destination, ageWindow string) (Decision, error)`
  - Extract lens from cluster.ClusterID (e.g., "list_lens:abc123" → "list_lens")
  - Support all decision types: "ignore", "watch", "cleanup"
  - Validate resulting Decision before return

- [x] **Mapping Test** (using MockStore)
  - Create sample `analysis.Cluster` with realistic data
  - Call `NewDecisionFromCluster()` with each decision type
  - Store via MockStore, retrieve and verify all fields
  - Test validation errors (invalid type, missing required fields for watch/cleanup)

**Deliverables:**
- `rulesgen/store_test.go` - Comprehensive SQLite storage tests ✅
- `rulesgen/mock_store.go` - In-memory store for unit testing (updated StoreInterface compliance) ✅
- Updated `rulesgen/types.go` - Add `NewDecisionFromCluster()` helper ✅
- `rulesgen/types_test.go` - Tests for Decision validation and cluster mapping ✅

**Note:** After comprehensive SQLite tests in `store_test.go`, use `mock_store.go` for all other unit tests to avoid SQLite overhead.

### Step 4: Render Analysis Output ✅
- [x] Update HTML template to display cluster information
- [x] Show cluster ID, lens, count, and examples
- [x] Basic table layout for cluster listing
- [x] Style the page for readability
- [x] Server runs analysis on page load and filters decided clusters
- [x] Template displays clusters with examples and lens badges
- [x] Responsive design with selection highlighting

### Step 5: Interactive Rule Creation ✅
- [x] Add action forms for each cluster (Ignore, Watch, Cleanup)
- [x] Implement POST endpoint for YAML preview (`/api/yaml-preview`)
- [x] Generate YAML rule previews from decisions  
- [x] Add YAML syntax highlighting with responsive design
- [x] Mobile-friendly interface with touch optimization
- [x] Copy and download functionality for generated YAML
- [x] Comprehensive testing of YAML generation logic

### Step 6: Load Existing Rules on Reload
- [ ] Load existing YAML rule files on server startup
- [ ] Parse existing rules into decision data type
- [ ] Store existing decisions in SQLite database
- [ ] Ensure decisions persist across server restarts

### Step 7: Filter Out Decided Clusters
- [ ] Query database for existing decisions
- [ ] Filter analysis results to exclude decided clusters
- [ ] Only show undecided clusters in the UI
- [ ] Add visual indicators for different decision types

## File Structure

```
postmanpat/
├── rulesgen/
│   ├── PLAN.md              # This document
│   ├── server.go            # HTTP server and handlers ✅
│   ├── analyzer.go          # Analysis wrapper ✅
│   ├── analyzer_test.go     # Unit tests for analyzer ✅
│   ├── types.go             # Core data structures ✅
│   ├── store.go             # SQLite storage layer ✅
│   ├── store_test.go        # Unit tests for storage (12 test cases) ✅
│   ├── mock_store.go        # In-memory store for unit testing ✅
│   ├── yaml_preview.go      # YAML generation functions ✅
│   ├── yaml_preview_test.go # YAML generation tests (125 tests) ✅
│   └── templates/
│       └── index.html       # Web UI template with YAML preview ✅
└── cli/
    ├── rulesgen.go          # Cobra command: "rulesgen serve" ✅
    └── rulesgen_test.go     # CLI tests ✅
```

## Data Models

### Decision (SQLite Storage)

```go
type Decision struct {
    ID          int64
    Lens        string    // list_lens, sender_unsub_lens, recipient_tag_lens
    ClusterID   string
    Type        string    // ignore, watch, cleanup
    Action      string    // delete, move
    Destination string    // for move action
    AgeWindow   string    // e.g., "30d", "7d", "1h"
    CreatedAt   time.Time
}
```

### Cluster (From Analysis)

```go
type Cluster struct {
    ID       string
    Lens     string
    Count    int
    Keys     map[string]interface{}
    Examples ClusterExamples
}

type ClusterExamples struct {
    SubjectRaw             []string
    Recipients             []string
    ReplyToDomains         []string
    SenderDomains          []string
    ReturnPathDomains      []string
    ListUnsubscribeTargets []string
}
```

### Rule Generation Input

```go
type ClusterDecision struct {
    Cluster   Cluster
    Decision  string   // ignore, watch, cleanup
    Action    string   // delete, move
    Destination string // for move action
    AgeWindow string   // for cleanup action
}
```

## SQLite Schema

```sql
-- Decisions table: stores all rule generation decisions
CREATE TABLE decisions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    lens TEXT NOT NULL,
    cluster_id TEXT NOT NULL,
    decision_type TEXT NOT NULL CHECK(decision_type IN ('ignore', 'watch', 'cleanup')),
    action TEXT CHECK(action IN ('delete', 'move')),
    destination TEXT,
    age_window TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(lens, cluster_id, decision_type)
);

-- Index for fast lookup
CREATE INDEX idx_decisions_lookup ON decisions(lens, cluster_id);
```

## UI Flow

### 1. Server Start
- Load environment variables
- Validate all required paths are set
- Open SQLite database connection
- Initialize schema if needed

### 2. Page Load (GET /)
- Run fresh analysis via `analyzer.go`
- Load all existing decisions from database
- Filter clusters: show only those without any decision
- Render HTML table with action forms for each cluster

### 3. User Actions per Cluster

| Decision | Description | Stored in DB |
|----------|-------------|--------------|
| **Ignore** | Permanently hide this cluster from future reviews | Yes |
| **Skip** | Skip for this session only, show again next time | No |
| **Watch** | Generate client-side watch rule | Yes |
| **Cleanup** | Generate server-side cleanup rule | Yes |

**Watch/Cleanup Options:**
- **Action**: Delete or Move
- **Destination**: Folder name (for Move action)
- **Age Window**: Duration string (e.g., "30d", "7d", "1h") - only for Cleanup

### 4. Generate (POST /api/generate)

Request body:
```json
{
  "decisions": [
    {
      "cluster_id": "list_lens:abc123",
      "lens": "list_lens",
      "decision": "watch",
      "action": "delete"
    },
    {
      "cluster_id": "sender_unsub_lens:def456",
      "lens": "sender_unsub_lens",
      "decision": "cleanup",
      "action": "move",
      "destination": "Archive",
      "age_window": "30d"
    }
  ]
}
```

Processing:
1. Validate each decision
2. Store decisions in SQLite
3. Generate YAML rule files:
   - `watch.yml` - Client matchers (regex-based)
   - `cleanup.yml` - Server matchers (substring-based) with age windows
   - `cleanup-onetime.yml` - Immediate cleanup rules (no age window)
4. Return success with file paths

### 5. Page Reload
- Fresh analysis run
- Filter out decided clusters
- Show only remaining undecided clusters

## Decision to Rule Mapping

| User Selection | Watch Rule | Cleanup Rule | Onetime Cleanup |
|---------------|-----------|--------------|-----------------|
| **Ignore** | - | - | - |
| **Watch + Delete** | client: {list_id_regex or sender_regex}, actions: [delete] | - | - |
| **Watch + Move** | client: {list_id_regex or sender_regex}, actions: [move] | - | - |
| **Cleanup + Delete** | - | server: {folders, list_id_substring or sender_substring, age}, actions: [delete] | server: {folders, list_id_substring or sender_substring}, actions: [delete] |
| **Cleanup + Move** | - | server: {folders, list_id_substring or sender_substring, age}, actions: [move] | server: {folders, list_id_substring or sender_substring}, actions: [move] |

**Lens-Specific Rule Building:**

**list_lens:**
- Watch: `list_id_regex` from ListID key
- Cleanup: `list_id_substring` from ListID key

**sender_unsub_lens:**
- Watch: `sender_regex` from SenderDomains + FromList keys
- Cleanup: `sender_substring` from SenderDomains key
- Optional: `replyto_regex` from ReplyToDomains examples
- Optional: `recipients_regex` from Recipients examples

**recipient_tag_lens:**
- Watch: `recipient_tag_regex` from recipient_tag key
- Cleanup: Not supported (shown in UI)

## Key Implementation Details

### Age Window Parsing
- Use existing `appconfig.ParseRelativeDuration()` function
- Supports: "30d", "7d", "1h", "24h"
- Store as string in DB, parse when generating rules

### Analysis Integration
- `analyzer.go` wraps `cli/analyze.go` logic
- Reuses existing cluster building functions
- Returns []Cluster instead of writing JSON file
- Runs fresh on every page load

### YAML Generation
- Use `gopkg.in/yaml.v3` (already in go.mod)
- Generate `appconfig.Rule` structs
- Write to configured output paths
- Preserve existing files (append or overwrite based on flag)

### SQLite Integration
- Add dependency: `modernc.org/sqlite` (pure Go, no CGO)
- Single connection per server instance
- Use transactions for batch inserts

### Web Framework
- Use Go standard `net/http` package
- Use `html/template` for UI rendering
- No external web framework needed
- Minimal JavaScript for form handling

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Serves main UI page |
| GET | `/api/clusters` | Returns JSON array of undecided clusters |
| POST | `/api/decisions` | Submit decisions (store in DB only) |
| POST | `/api/generate` | Submit decisions and generate YAML files |

## Testing Strategy

### Unit Tests

**store_test.go:**
- Test CreateDecision, GetDecisions, HasDecision
- Test duplicate prevention (UNIQUE constraint)
- Use in-memory SQLite database

**generator_test.go:**
- Test decision to YAML conversion for each lens
- Test rule structure matches expected format
- Test age window parsing

**analyzer_test.go:**
- Test cluster extraction from MailData
- Test lens-specific key building

### Integration Tests

**server_test.go:**
- Test HTTP handlers with test server
- Test full flow: clusters → decisions → YAML
- Test environment variable validation

## Dependencies to Add

```go
// In go.mod
modernc.org/sqlite v1.28.0  // Pure Go SQLite driver
```

## Migration Path from Python

1. The Python script's checkpoint JSON format is different from the new SQLite format
2. Users will start fresh with the new system
3. Old checkpoint files can be manually migrated if needed
4. The decision store is additive - it doesn't need historical data to function

## Future Enhancements

**Completed Analysis:**
- **LLM Model Selection Analysis**: Detailed evaluation of 3 top OpenRouter models for coding capability vs cost. Recommended `google/gemini-2.0-flash-exp` as most capable for coding tasks.

**Implementation Ready:**
1. **Rule Editing**: Allow editing existing rules
2. **Bulk Actions**: Apply same decision to multiple clusters  
3. **Search/Filter**: Filter clusters by keywords
4. **Statistics Dashboard**: Show decision counts, rule statistics
5. **Rule Preview**: ✅ **Implemented** - YAML preview with syntax highlighting
6. **Undo**: Remove decisions and regenerate rules
7. **LLM Integration**: Use Gemini-2.0-Flash-Exp for rule suggestions

## Security Considerations

1. **File Paths**: All file paths come from env vars, validated at startup
2. **SQL Injection**: Use parameterized queries throughout
3. **XSS Prevention**: Proper HTML escaping in templates
4. **CSRF**: Consider CSRF tokens for POST endpoints if exposed externally

## Error Handling

1. **Missing Env Vars**: Fatal error on startup with clear message
2. **DB Errors**: 500 error with logging, user-friendly message
3. **Invalid Decisions**: 400 error with validation details
4. **YAML Write Errors**: 500 error with file path details
5. **Analysis Errors**: Show error in UI, allow retry
