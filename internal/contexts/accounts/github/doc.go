// Package github talks to GitHub for accounts. Client signs visitors in with a GitHub OAuth app: it sends them to
// GitHub to authorize Rulemart, then exchanges the code GitHub returns for a token and reads who they are with it. It
// asks for read:org, so the token also reads the organizations the visitor belongs to, and public repositories, which
// the dashboard reads. The flow uses PKCE, so a code that leaks on its way back is worthless without the verifier the
// browser kept. API reads a visitor's organizations and repositories, App acts as the GitHub App that reads private
// repositories where visitors install it, and App.WebhookInstallation checks and reads the deliveries of that app's webhook.
package github
