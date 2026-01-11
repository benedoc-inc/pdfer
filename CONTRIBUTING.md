# Contributing to pdfer

Thank you for your interest in contributing to pdfer! This document provides guidelines and information for contributors.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/pdfer.git`
3. Create a branch: `git checkout -b feature/your-feature-name`

## Development Setup

```bash
# Install dependencies (none required - pure Go!)
go mod download

# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests verbosely
go test -v ./...
```

## Code Style

- Follow standard Go conventions (use `gofmt`)
- Add GoDoc comments to all exported types and functions
- Keep functions focused and small
- Use meaningful variable names

## Testing

- Add tests for all new functionality
- Maintain or improve code coverage
- Tests should be in `*_test.go` files alongside the code
- Integration tests go in the `tests/` directory

### Running Specific Tests

```bash
# Run tests for a specific package
go test -v ./pdf/encryption/...

# Run a specific test
go test -v -run TestDecryptObject ./pdf/encryption/...
```

## Pull Request Process

1. Ensure all tests pass
2. Update documentation if needed
3. Add a clear description of changes
4. Reference any related issues

## Commit Messages

Use clear, descriptive commit messages:

```
feat: add AES-256 encryption support
fix: handle empty xref streams correctly
docs: update README with new examples
test: add tests for object stream parsing
refactor: simplify key derivation logic
```

## Areas for Contribution

See [GAPS.md](GAPS.md) for a comprehensive list of implementation gaps with:
- Priority levels
- Complexity estimates
- Code snippets to get started
- Links to relevant specs

### High Priority
1. Incremental updates parsing
2. Font embedding
3. Image embedding
4. Page content streams
5. AES-256 full support

### Good First Issues
- Add ASCIIHexDecode filter (simple)
- Improve error messages
- Add more test cases
- Documentation improvements

## Questions?

Feel free to open an issue for:
- Bug reports
- Feature requests
- Questions about the codebase
- Help with implementation

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
