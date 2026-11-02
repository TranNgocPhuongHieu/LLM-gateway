# Security Policy

## Reporting a vulnerability

Please report security issues privately to the repository owner through GitHub:

<https://github.com/TranNgocPhuongHieu/LLM-gateway/security/advisories/new>

Do not post API keys, session secrets, database credentials, provider keys, or
full request logs in a public issue. Include a minimal reproduction, affected
version or commit, impact, and any safe mitigation you have verified.

## Secret handling

- Keep `.env`, database files, logs, provider credentials, and private keys out
  of Git.
- Rotate any credential that has appeared in a terminal, issue, screenshot, or
  public commit.
- Use repository or environment secrets for CI; never hard-code credentials in
  source, tests, fixtures, or documentation.
