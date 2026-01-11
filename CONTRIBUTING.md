# Contributing to pdfer

Thank you for your interest in contributing to pdfer! This document provides guidelines and information for contributors.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/pdfer.git`
3. Create a branch: `git checkout -b feature/your-feature-name`

## Development Setup

```bash
# Run the setup script (configures git hooks and verifies build)
./scripts/setup.sh

# Or set up manually:
git config core.hooksPath .githooks  # Enable pre-commit and pre-push hooks
go mod download                       # Install dependencies
go test ./...                         # Run tests
```

### Git Hooks

This project uses git hooks to maintain code quality:

- **pre-commit**: Runs `gofmt` and `go vet` on staged files
- **pre-push**: Runs the full test suite before pushing

The setup script configures these automatically. To skip hooks temporarily:
```bash
git commit --no-verify  # Skip pre-commit
git push --no-verify    # Skip pre-push
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
1. Font embedding (TrueType/OpenType subsetting)
2. AES-256 full support
3. Cross-reference stream writing
4. Linearized PDF support

### Good First Issues
- Add LZWDecode filter
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
