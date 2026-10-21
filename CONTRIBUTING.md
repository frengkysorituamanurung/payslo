# Contributing to PaySLO Lab

Thank you for your interest in contributing. PaySLO Lab exists to onboard site reliability engineers,
so simplicity and clarity take priority over cleverness. Read [AGENTS.MD](AGENTS.MD) before you begin;
it defines the working rules, the technology stack, and design principles P1 to P8.

Local setup is described in [docs/RUNNING.md](docs/RUNNING.md). For UI-specific guidance, see
[ui/CONTRIBUTING.md](ui/CONTRIBUTING.md).

## Developer Certificate of Origin

Every commit must carry a `Signed-off-by` line, certifying the Developer Certificate of Origin. Use
`git commit -s`:

```sh
git commit -s -m "feat(gateway): validate amount range"
```

The resulting footer looks like this:

```text
Signed-off-by: Your Name <you@example.com>
```

Pull requests without a sign-off are rejected by CI, which checks
`git log --format=%B | grep Signed-off-by`.

## Workflow

1. Work only on the phase or scope you were asked to address, as required by rule 1 in `AGENTS.MD`.
2. Keep one change per commit and use [Conventional Commits](https://www.conventionalcommits.org/)
   prefixes such as `feat:`, `fix:`, `docs:`, `chore:`, and `test:`.
3. Write tests together with the code. A phase is not complete until its tests pass.
4. Run the relevant gates before pushing:

   ```sh
   make fmt
   make lint
   make test
   ```

   For UI changes, also run:

   ```sh
   make ui-test
   make ui-e2e
   make ui-audit
   ```

5. Do not add a dependency without recording it in [docs/DECISIONS.md](docs/DECISIONS.md), including
   the reason and the estimated impact on build time.
6. Never commit secrets or real data. All data in this lab is synthetic; see the security notice in
   the [README](README.md).

## Reporting issues

A useful report includes:

- The command you ran and its full output, quoted rather than paraphrased.
- The versions of Docker, Rust, Node.js, Go, and your operating system, as applicable.
- The smallest sequence of steps that reproduces the problem.
- Whether the problem occurs in `make up-lite`, `make up-prebuilt`, or `make serve`.

Beginner-friendly issues are listed in [docs/GOOD_FIRST_ISSUES.md](docs/GOOD_FIRST_ISSUES.md).

## Pull requests

- Keep the change focused and describe the behavior it adds or fixes.
- Reference the issue it addresses when one exists.
- Confirm that the quality gates above pass, and include the commands and results in the description.
- Update documentation and the changelog when behavior changes.
- Expect review comments about scope, tests, and consistency with the existing structure.

## Code and documentation conventions

- Rust code must pass `cargo fmt` and `cargo clippy --workspace --all-targets -- -D warnings`.
- Shell scripts use `set -euo pipefail` and must pass `shellcheck`.
- User-facing text in the Lab Console is internationalized in both Indonesian and English locales.
- Documentation is written in clear, professional prose. Avoid emoji and unnecessary formatting.
- Money is always an integer in the smallest IDR unit; never use floating-point types for money.
- Intentional lab flaws are marked with `// LAB-FLAW:` comments and documented in the mentor guide
  rather than hidden.
