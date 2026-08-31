# Contributing to MIRA

Thank you for your interest in contributing to MIRA! This document outlines the conventions, testing requirements, and processes for contributing.

## Table of Contents

1. [Code of Conduct](#code-of-conduct)
2. [Development Setup](#development-setup)
3. [Coding Conventions](#coding-conventions)
4. [Testing Requirements](#testing-requirements)
5. [Branch & Versioning](#branch--versioning)
6. [Migration Process](#migration-process)
7. [Pull Request Process](#pull-request-process)
8. [License](#license)

---

## Code of Conduct

Please read and follow the [Code of Conduct](CODE_OF_CONDUCT.md) to ensure a welcoming environment for all contributors.

---

## Development Setup

### Prerequisites

- Go 1.23+
- GCC (for CGO, `go-sqlite3`)
- Make

### Building

```bash
# Clone the repository
git clone https://github.com/benoitpetit/mira.git
cd mira

# Build the binary
go build -o mira ./cmd/mira

# Or use Make
make build
```

### Running Tests

```bash
go test ./...
```

### Linting

```bash
# Run golangci-lint
golangci-lint run ./...
```

Or use Make:
```bash
make lint
```

---

## Coding Conventions

### General

- Follow [Go SOP](https://go.dev/doc/effective_go) guidelines
- Use `gofmt` for code formatting (`gofmt -w .`)
- All new code must compile and pass `go vet`

### Naming

- Package names: lowercase, single words (e.g., `config`, `usecases`, `storage`)
- Struct names: PascalCase (e.g., `AllocatorConfig`, `RecallMemory`)
- Function names: camelCase for exports, lowercase for internals
- Constants: UPPER_SNAKE_CASE (e.g., `MaxCandidates`, `DefaultBudget`)
- File names: lowercase with optional suffix (e.g., `config.go`, `sqlite_repository.go`)

### Error Handling

- Always check errors and handle them appropriately
- Use `fmt.Errorf` with `%w` for wrapping errors
- Don't ignore errors silently (see `recall_memory.go` FTS5 error handling as example of improvement)

### Documentation

- Document all public types and functions with godoc comments
- Include examples in godoc comments where helpful
- Update `config.example.yaml` if adding new config fields

---

## Testing Requirements

### Unit Tests

- All new functions must have unit tests
- Tests should be in the same package (`_test.go` files)
- Follow the existing test patterns in the codebase
- Run `go test ./...` to verify all tests pass

### Test Coverage

- Aim for at least 90% test coverage on new code
- Critical paths (storage, vector search, recall) must have full coverage

### Testing Patterns

- Use table-driven tests for functions with multiple input combinations
- Use `subtest` names for better error reporting
- Mock dependencies using interfaces when testing internal logic

---

## Branch & Versioning

### Branching Model

- `main`: Production-ready branch. Contains stable releases only.
- `dev`: Development branch. Latest features and changes.
- Feature branches: `feature/short-description` branched from `dev`
- Bugfix branches: `fix/short-description` branched from `dev` or `main`

### Versioning

- Follow [Semantic Versioning](https://semver.org/) (MAJOR.MINOR.PATCH)
- Releases are cut from `main` branch
- Use `go run ./cmd/mira version` or check `git tag` for version

### Branch Workflow

1. Ensure you're on `dev` or create a feature branch: `git checkout -b feature/description dev`
2. Make your changes
3. Run tests: `go test ./...`
4. Run lint: `make lint`
5. Commit with descriptive message following [Conventional Commits](https://www.conventionalcommits.org/):
   - `feat: add new feature`
   - `fix: bug fix`
   - `docs: documentation update`
   - `refactor: code refactor`
   - `test: add missing tests`
   - `chore: maintenance task`
6. Push and create Pull Request targeting `dev`

---

## Migration Process

### Adding New Migrations

When adding new PostgreSQL migrations:

1. Create a new migration file in `migrations_postgres/` following the naming convention: `YYYYMMDD_description.sql`
2. Ensure the migration is idempotent where possible
3. Test the migration against a fresh database and an existing database
4. Update `postgres_repository.go` to include the new migration
5. Add migration steps to the `migrate` command in `cmd/mira/main.go`

### SQLite Migrations

- SQLite schema evolves automatically through Go schema changes
- Add new columns/tables in Go struct definitions
- Run `go build && ./mira migrate` to apply changes
- Backward compatibility is maintained through the Go code

---

## Pull Request Process

1. **Fork the repository** and create your feature branch
2. **Run the full test suite**: `go test ./...`
3. **Run the linter**: `make lint`
4. **Update documentation** if needed (SKILL.md, README.md, config.example.yaml)
5. **Ensure code follows conventions** (gofmt, godoc, naming)
6. **Create a Pull Request** targeting the `dev` branch
7. **Fill the PR template** with:
   - Description of changes
   - Motivation for the changes
   - Any breaking changes
   - Test results
8. **Address reviewer feedback** and push additional commits
9. **Merge** once approved (squash and merge preferred)

### PR Checklist

- [ ] All tests pass (`go test ./...`)
- [ ] Linting passes (`make lint`)
- [ ] Code follows project conventions
- [ ] Documentation updated if needed
- [ ] No breaking changes (or documented breaking changes)
- [ ] Commit messages follow Conventional Commits

---

## License

By contributing to MIRA, you agree that your contributions will be licensed under the [MIT License](LICENSE).

See the [LICENSE](LICENSE) file for full details.