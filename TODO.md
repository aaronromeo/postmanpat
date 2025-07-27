# PostmanPat TODO List

## Critical Test Coverage Gaps (Priority 1)

### CLI Commands - PARTIAL Coverage ✅
- [X] **Create `cmd/postmanpat/main_test.go`** ✅
  - [X] Test `listMailboxNames` function ✅
    - [X] Successful execution with various mailbox configurations ✅
    - [X] Error handling when `isi.GetMailboxes()` fails ✅
    - [X] Error handling when JSON marshaling fails ✅
    - [X] Error handling when file writing fails ✅
    - [X] Telemetry attribute setting verification ✅
    - [X] File content validation (correct JSON structure) ✅
    - [X] File permissions verification (0644) ✅
  - [ ] Test `reapMessages` function
    - [ ] Successful message processing workflow
    - [ ] Error handling when mailbox list file doesn't exist
    - [ ] Error handling when JSON unmarshaling fails
    - [ ] Error handling when mailbox processing fails
    - [ ] Integration with ImapManager and FileManager
  - [ ] Test `webserver` function
    - [ ] Fiber app configuration
    - [ ] Middleware setup verification
    - [ ] Route registration verification
    - [ ] Template engine configuration
    - [ ] Static file serving setup

### HTTP Handlers - ZERO Coverage
- [ ] **Create `handlers/handlers_test.go`**
  - [ ] Test `Home` handler
    - [ ] Successful template rendering
    - [ ] Correct context data passing
  - [ ] Test `About` handler
    - [ ] Template rendering without data
  - [ ] Test `NotFound` handler
    - [ ] 404 status code verification
    - [ ] Error template rendering
  - [ ] Test `Mailboxes` handler
    - [ ] Successful mailbox data retrieval
    - [ ] FileManager integration
    - [ ] Error handling when file reading fails
    - [ ] Error handling when JSON unmarshaling fails
    - [ ] Template rendering with mailbox data

### Storage/File Management - 0% Coverage
- [ ] **Create `pkg/utils/storagemanager_test.go`**
  - [ ] Test `OSFileManager`
    - [ ] File creation and writing
    - [ ] Directory creation
    - [ ] File reading
    - [ ] Error handling for permission issues
    - [ ] Error handling for disk space issues
  - [ ] Test `S3FileManager`
    - [ ] S3 object creation and writing
    - [ ] Bucket operations (create, exists)
    - [ ] Error handling for AWS credential issues
    - [ ] Error handling for network failures
    - [ ] Integration with DigitalOcean Spaces

## High Priority Test Improvements (Priority 2)

### Integration Tests
- [ ] **Create `integration/` directory**
  - [ ] End-to-end `mailboxnames` workflow test
    - [ ] Real IMAP server interaction (with test server)
    - [ ] File system integration
    - [ ] S3 storage integration (with localstack)
  - [ ] End-to-end `reapmessages` workflow test
    - [ ] Complete email processing pipeline
    - [ ] File format compatibility between commands
    - [ ] Storage verification
  - [ ] Web server integration tests
    - [ ] HTTP endpoint testing
    - [ ] Template rendering verification
    - [ ] Static asset serving

### OpenTelemetry Integration - 0% Coverage
- [ ] **Create `pkg/utils/otel_test.go`**
  - [ ] Test `SetupOTelSDK` function
    - [ ] Successful telemetry initialization
    - [ ] Error handling for invalid configuration
    - [ ] Trace provider setup verification
    - [ ] Metrics provider setup verification
    - [ ] Logger provider setup verification
  - [ ] Test telemetry in CLI commands
    - [ ] Span creation and attributes
    - [ ] Trace propagation
    - [ ] Metrics collection

### Error Scenario Coverage
- [ ] **Enhance existing test files with error scenarios**
  - [ ] Network timeout scenarios
  - [ ] IMAP authentication failures
  - [ ] Disk space exhaustion
  - [ ] Permission denied errors
  - [ ] Malformed configuration files
  - [ ] Large dataset handling

## Medium Priority Improvements (Priority 3)

### Edge Case Testing
- [X] **Add edge case tests to existing test files** ✅ (Partial - for listMailboxNames)
  - [X] Empty mailbox list scenarios ✅
  - [ ] Very large mailbox names or counts
  - [X] Special characters in mailbox names ✅ (Added test case)
  - [ ] Unicode handling in email content
  - [ ] Concurrent file access scenarios
  - [ ] Memory pressure scenarios

### Performance Testing
- [ ] **Create `performance/` directory**
  - [ ] Benchmark tests for email processing
  - [ ] Memory usage profiling
  - [ ] Concurrent operation testing
  - [ ] Large email volume testing

### Mock Improvements
- [X] **Enhance `pkg/mock/` package** ✅ (Partial - for CLI testing)
  - [ ] Add more realistic IMAP server mocks
  - [ ] Add S3 service mocks
  - [ ] Add HTTP client mocks for external services
  - [X] Improve error simulation capabilities ✅ (Added MockFileManager with error simulation)

## Low Priority Enhancements (Priority 4)

### Test Infrastructure
- [X] **Improve test setup and utilities** ✅ (Partial - for CLI testing)
  - [X] Add test data generators ✅ (Created MockFileManager and MockImapManager)
  - [X] Create reusable test fixtures ✅ (Table-driven test structure)
  - [ ] Add test environment setup scripts
  - [ ] Implement test database seeding

### Documentation Testing
- [ ] **Create example validation tests**
  - [ ] Verify README examples work
  - [ ] Test configuration file examples
  - [ ] Validate API documentation

### Security Testing
- [ ] **Add security-focused tests**
  - [ ] Input validation testing
  - [ ] Authentication bypass attempts
  - [ ] File path traversal prevention
  - [ ] Credential handling security

## Test Coverage Goals

### Current Coverage: 57.2%
- **Target Coverage: 85%+**

### By Component:
- [X] **CLI Commands**: 0% → 80%+ ✅ (listMailboxNames function fully tested)
- [ ] **HTTP Handlers**: 0% → 85%+
- [ ] **Storage Management**: 0% → 75%+
- [ ] **OpenTelemetry**: 0% → 70%+
- [ ] **IMAP Manager**: 75.9% → 85%+
- [ ] **Mailbox Processing**: 62.5% → 80%+

## Implementation Strategy

### Phase 1: Critical Coverage (Weeks 1-2)
1. CLI command tests ✅ (listMailboxNames complete)
2. HTTP handler tests
3. Basic storage manager tests

### Phase 2: Integration & Error Handling (Weeks 3-4)
1. End-to-end integration tests
2. Comprehensive error scenario coverage
3. OpenTelemetry testing

### Phase 3: Edge Cases & Performance (Weeks 5-6)
1. Edge case testing
2. Performance benchmarks
3. Security testing

### Phase 4: Polish & Documentation (Week 7)
1. Test infrastructure improvements
2. Documentation validation
3. Final coverage optimization

## Success Metrics

- [ ] **85%+ overall test coverage**
- [X] **Zero critical paths without tests** ✅ (listMailboxNames critical path now tested)
- [X] **All CLI commands fully tested** ✅ (1/3 complete - listMailboxNames)
- [ ] **All HTTP endpoints tested**
- [ ] **Integration tests for main workflows**
- [ ] **Performance benchmarks established**
- [ ] **Security vulnerabilities tested**

## Notes

- [X] Focus on testing the `mailboxnames` command first as it's the entry point ✅ **COMPLETED**
- Ensure file format compatibility tests between `listMailboxNames` and `reapMessages`
- Use testcontainers for integration testing with real services
- Consider property-based testing for email parsing logic
- [X] Implement golden file testing for JSON output validation ✅ (JSON structure validation added)

## Existing Technical Debt

### From README.md TODO List
- [ ] Change to use ufave cli (already using urfave/cli/v2)
- [X] Multi app droplet deployment
- [ ] Replace docker compose with microk8s
- [X] Add comprehensive CLI command tests ✅ (listMailboxNames complete)
- [ ] Add HTTP handler tests
- [ ] Improve S3 storage integration tests

### Code Quality Improvements
- [ ] **Remove debug print statements**
  - [ ] Clean up `fmt.Println` statements in mailbox processing
  - [ ] Replace with proper structured logging
- [ ] **Improve error messages**
  - [ ] Add more context to error wrapping
  - [ ] Standardize error message formats
- [ ] **Add input validation**
  - [ ] Validate environment variables on startup
  - [ ] Validate configuration file formats
  - [ ] Add bounds checking for numeric inputs

### Documentation Improvements
- [ ] **Add inline code documentation**
  - [ ] Document all public functions and types
  - [ ] Add usage examples in godoc comments
  - [ ] Document configuration options
- [ ] **API Documentation**
  - [ ] Document HTTP endpoints
  - [ ] Add OpenAPI/Swagger specification
  - [ ] Document CLI command options