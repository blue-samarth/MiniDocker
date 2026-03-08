# Test Suite for miniDocker

## Running Tests

### Unit Tests
Run all unit tests without requiring root privileges:
```bash
cd tests
go test -v -run "Test(InitProcess|RunContainer|CmdRun)"
```

### Integration Tests
Run integration tests (requires root):
```bash
cd tests
sudo go test -v -run "TestPhase1"
```

### All Tests
```bash
cd tests
sudo go test -v
```

### Short Mode
Skip long-running integration tests:
```bash
go test -short -v
```

## Test Organization

- `init_test.go` - Unit tests for container init process
- `run_test.go` - Unit tests for container run function
- `cmd_test.go` - Unit tests for command layer
- `integration_test.go` - End-to-end integration tests

## Phase 1 Tests

### Unit Tests
- Argument validation
- Error handling
- Function contracts

### Integration Tests
- Basic execution flow
- Hostname isolation
- Signal handling
- Namespace creation

## Notes

- Integration tests require root privileges
- Some tests require a proper Linux environment
- Tests may be skipped on unsupported platforms
- Use `-v` flag for verbose output
