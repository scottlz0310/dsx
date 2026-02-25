# Contributing Guide

Thank you for your interest in contributing to this project! 🎉

## 🚀 Getting Started

### Prerequisites

- Go 1.25 or higher
- Git

### Development Setup

1. **Fork and clone the repository**
   ```bash
   git clone https://github.com/scottlz0310/dsx.git
   cd dsx
   ```

2. **Install dependencies**
   ```bash
   go mod tidy
   ```

3. **Verify the setup**
   ```bash
   # Run tests
   go test ./...

   # Run CI simulation
   # (Assuming you have golangci-lint installed)
   golangci-lint run
   ```

## 🔄 Development Workflow

### 1. Create a Branch

```bash
git checkout -b feature/your-feature-name
# or
git checkout -b fix/your-bug-fix
```

### 2. Make Changes

- Write your code following the project's style guidelines
- Add tests for new functionality
- Update documentation as needed
- Ensure all tests pass

### 3. Commit Changes

We use [Conventional Commits](https://www.conventionalcommits.org/):

```bash
git commit -m "feat: add new feature"
git commit -m "fix: resolve bug in module"
git commit -m "docs: update README"
git commit -m "test: add unit tests for feature"
```

### 4. Push and Create PR

```bash
git push origin your-branch-name
```

Then create a Pull Request on GitHub.

## 📝 Code Style

### Python Code Style

We use the standard Go coding style.

- **Check**: `go vet ./...`
- **Lint**: `golangci-lint run`
- **Format**: `gofmt -s -w .`

### Style Guidelines

- Follow PEP 8 style guide
- Use type hints for all functions and methods
- Write docstrings for all public functions, classes, and modules
- Keep line length to 120 characters (modern standard)
- Use meaningful variable and function names

### Example

```python
from typing import List, Optional

def process_data(
    items: List[str],
    filter_empty: bool = True
) -> Optional[List[str]]:
    """Process a list of string items.

    Args:
        items: List of strings to process
        filter_empty: Whether to filter out empty strings

    Returns:
        Processed list of strings, or None if input is empty

    Raises:
        ValueError: If items contains non-string elements
    """
    if not items:
        return None

    if filter_empty:
        items = [item for item in items if item.strip()]

    return [item.strip().lower() for item in items]
```

## 🧪 Testing

### Running Tests

```bash
# Run all tests
uv run pytest

# Run with coverage
uv run pytest --cov=. --cov-report=html

# Run specific test file
uv run pytest tests/test_module.py

# Run tests matching a pattern
uv run pytest -k "test_feature"
```

### Writing Tests

- Write tests for all new functionality
- Use descriptive test names
- Follow the Arrange-Act-Assert pattern
- Use pytest fixtures for common setup

### Example Test

```python
import pytest
from mymodule import process_data

class TestProcessData:
    def test_process_data_with_valid_input(self):
        # Arrange
        items = ["  Hello  ", "World", ""]

        # Act
        result = process_data(items, filter_empty=True)

        # Assert
        assert result == ["hello", "world"]

    def test_process_data_with_empty_list(self):
        # Arrange
        items = []

        # Act
        result = process_data(items)

        # Assert
        assert result is None
```

## 📚 Documentation

### Docstring Style

We use Google-style docstrings:

```python
def example_function(param1: str, param2: int = 0) -> bool:
    """Example function with Google-style docstring.

    Args:
        param1: The first parameter
        param2: The second parameter (default: 0)

    Returns:
        True if successful, False otherwise

    Raises:
        ValueError: If param1 is empty
        TypeError: If param2 is not an integer
    """
    pass
```

### README Updates

When adding new features:
- Update the README.md with usage examples
- Add new features to the feature list
- Update installation instructions if needed

## 🔒 Security

### Security Guidelines

- Never commit secrets, API keys, or passwords
- Use environment variables for sensitive configuration
- Run security scans before submitting PRs
- Follow secure coding practices

### Security Tools

```bash
# Run security linting
uv run bandit -r .

# Check for secrets
uv run detect-secrets scan --all-files

# Check dependencies for vulnerabilities
uv run safety check
```

## 🐛 Bug Reports

When reporting bugs, please include:

- Python version
- Operating system
- Steps to reproduce
- Expected vs actual behavior
- Error messages or logs
- Minimal code example

## ✨ Feature Requests

When requesting features:

- Describe the problem you're trying to solve
- Explain why this feature would be useful
- Provide examples of how it would be used
- Consider if it fits the project's scope

## 📋 Pull Request Checklist

Before submitting a PR, ensure:

- [ ] Code follows the style guidelines
- [ ] Tests are written and passing
- [ ] Documentation is updated
- [ ] Commit messages follow conventional commits
- [ ] PR description explains the changes
- [ ] Security considerations are addressed
- [ ] Breaking changes are documented

## 🤝 Code Review Process

1. **Automated Checks**: All PRs must pass CI checks
2. **Peer Review**: At least one maintainer must review
3. **Security Review**: Security-related changes need security team review
4. **Testing**: New features must include tests
5. **Documentation**: User-facing changes need documentation updates

## 🏷️ Release Process

1. Update version in `pyproject.toml`
2. Update `CHANGELOG.md`
3. Create a release PR
4. Tag the release after merging
5. GitHub Actions will automatically publish to PyPI

## 💬 Getting Help

- **GitHub Discussions**: For questions and general discussion
- **GitHub Issues**: For bug reports and feature requests
- **Email**: For security-related concerns

## 🙏 Recognition

Contributors will be recognized in:
- `CONTRIBUTORS.md` file
- Release notes
- GitHub contributors page

Thank you for contributing! 🎉
