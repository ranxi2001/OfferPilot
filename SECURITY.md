# Security Policy

## Supported Versions

Security fixes are handled on the `main` branch.

## Sensitive Data

OfferPilot may process provider API keys, resumes, job descriptions, audio
recordings, transcripts, and interview reports. Do not commit `.env`,
`data/agent.db`, generated reports, private resumes, recordings, or logs that
contain secrets or personal data.

For production deployment, set a strong `OFFERPILOT_API_KEY`, restrict
`OFFERPILOT_ALLOWED_ORIGINS`, keep `OFFERPILOT_ENABLE_CONFIG_API=false`, and
store provider keys in the server environment rather than browser storage.

## Reporting

If you find a vulnerability, open a private report to the repository owner or
contact the maintainer directly. Do not publish secret-bearing examples,
provider keys, private resumes, audio, or unredacted logs in a public issue.
