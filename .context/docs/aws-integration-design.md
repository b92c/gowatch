# Phase 4, first delivery: AWS accounts and CloudWatch Logs

Date: 2026-10-25. Branch: `feat/aws-integration`. Roadmap items: Phase 4 foundation and P4.1 (CloudWatch Logs). P4.2 (Lambda), P4.3 (X-Ray) and P4.4 (CloudFormation) come later on top of this work.

## 1. Intent and why

The AWS view (`a`) has been empty since it exists. `internal/aws` is a skeleton: no AWS SDK in `go.mod`, four collectors that return fixed sample data, and nothing outside the tests that calls them or fills the state. The only thing the view shows is a region string, and that string is a hard-coded fallback.

The owner logs in to AWS through IAM Identity Center (SSO). On this machine the `[default]` profile in `~/.aws/config` is a legacy SSO profile (`sso_start_url`, `sso_region`, `sso_account_id`, `sso_role_name`, `sso_registration_scopes`, `region`), a valid SSO token sits in `~/.aws/sso/cache`, the `[default]` section of `~/.aws/credentials` holds temporary keys exported by the owner's login tooling, and no `AWS_*` environment variable is set. The owner's words for what the view should show first: "as minhas contas atreladas à AWS e os logs do CloudWatch". That is:

1. The AWS accounts linked to that login (the accounts visible in the SSO portal after `aws sso login`), with their connection state.
2. CloudWatch Logs: the log groups, and the recent events of the followed groups inside the Logs panel, next to the Docker logs.

Everything else in Phase 4 waits.

## 2. Goals, non-goals, anti-goals

### Goals

- G1. A real AWS client using the AWS SDK for Go v2 and its default credential chain, so SSO, shared config and credentials files, environment variables, web identity, container and instance roles all work without GoWatch-specific credential code.
- G2. Accounts: always show the identity of the active credentials (account ID, ARN). When the resolved profile is SSO-based, list every account and role reachable with the cached SSO token. The identity's account is monitored by default; other SSO accounts are listed as available and can be monitored on request.
- G3. CloudWatch Logs: list the log groups of every monitored account in the resolved region; tail the followed groups and show their events in the Logs panel as log lines whose "service" is the log group, so the search, the minimum log level, the export (`e`) and the trace correlator work on them unchanged.
- G4. The AWS view always says what is going on: not configured, disabled, connecting, connected as `<account>`, or the last error with a hint (for example an expired SSO token and the advice to run `aws sso login`). An empty table with no explanation is a bug.
- G5. The AWS work runs in its own goroutine with its own interval. It never blocks startup, the TUI or the Docker refresh loop (2 s by default).
- G6. Bounded API usage by default: a few calls per interval, hard caps on followed groups and on events per poll, and no call at all when AWS is not configured.
- G7. Everything configurable from the YAML file (and the existing `AWS_*` variables), plus, as the last stage, an interactive way to follow a log group or monitor an account from the AWS table.
- G8. Tests without network. All CI gates green: `gofmt`, `go vet`, `go mod tidy -diff`, `fieldalignment`, `gosec`, `go test -race`, cross build, Docker build, `govulncheck`. Documentation updated (README, example config, roadmap, context docs).

### Non-goals (this delivery)

- Lambda metrics or Lambda-specific views (P4.2), X-Ray (P4.3), CloudFormation (P4.4). The foundation must leave room for them (a collector per service, a resource type per row), nothing more.
- More than one region at a time. Only the resolved region is used.
- CloudWatch Logs Insights queries, metric filters, subscription filters, creating or changing anything in AWS. The integration is read-only.
- The `StartLiveTail` streaming API. Polling `FilterLogEvents` is enough for this delivery.
- Monitoring extra accounts without SSO (for example through a list of named profiles). With plain credentials only the caller's account is monitored; `aws.accounts` entries are then reported as "requires SSO".
- Alert rules on AWS data, notifications, AWS data in the System Resources panel.
- Showing AWS data while the Docker daemon is offline (the dashboard shows the daemon error page then; unchanged).

### Anti-goals (a failure even if it works)

- Any AWS call when nothing configures AWS, or any network attempt during startup that delays the TUI.
- Unbounded calls: tailing every log group of every account, or polling faster than the configured interval.
- A credential, SSO token or secret in a log line, an error text, the export file or a test fixture.
- A different look for the `dark` theme in the Docker view, or a different behavior for any existing default key. The only keyboard differences allowed against the old `handleInput` are the two new keys of stage 4 (`Tab`, `Enter`), registered in `input_oracle_test.go` exactly as `e`/`E` were. In `legacy_look_test.go` only the subtest `aws resources` of `TestDarkThemeTableColorsMatchTheOriginal` changes, because it feeds the old `AWSResource` rows; it is adapted to the new row model with the same literal colors and the same rule (section 4.6). The rest of that file is untouched.
- Behavior changes for users without AWS. Without a config file and without AWS files or variables, GoWatch must behave exactly as today, except that the AWS view explains that it is not configured.

## 3. Constraints

Project rules that bind this work (see `AGENTS.md`, `.context/agents/feature-developer.md`, `.context/docs/project-roadmap.md`):

- Go 1.27. Direct dependencies today: tcell, tview, moby client/api, go.yaml.in/yaml/v3. The AWS SDK modules become direct dependencies; `go mod tidy -diff` must be clean, `govulncheck` clean, and the cross build (several GOOS/GOARCH) must still work. The SDK is pure Go, so cgo is not a concern; the binary gets larger, which is accepted.
- `internal/config` is a leaf package: standard library, the YAML decoder, `internal/alert` and `pkg/metrics` only. It must not import `internal/aws`.
- Dependency direction: `internal/docker` imports `internal/aws`; `internal/aws` must not import `internal/docker`, `internal/ui` or `internal/filter`. `internal/filter` imports `internal/docker`.
- The working-directory `./gowatch.yaml` is untrusted (it may come from a cloned repository). `aws.endpoint_url` is already refused from that layer (`internal/config/untrusted.go`). See section 4.9 for the new settings.
- No color literals in `internal/ui`; colors come from `Theme` roles. A new color is a new role in all four themes plus README and `DefaultYAML()`. This delivery needs no new role: the existing `StatusOK`, `StatusWarn`, `StatusError`, `Muted`, `Text`, `Info` and `Accent` cover every AWS cell.
- Keys are never matched in code; a new key is a new `Action` with its `actionTable` entry, default preset keys, `runAction` case, README row and `DefaultYAML()` line. The default preset must keep the old behavior for the old keys. `input_oracle_test.go` records the return value and the state (view, modal, filters, focus, page, scrolling) after every key in every state and compares them with a verbatim copy of the old `handleInput`; a new key that the old code passed through is registered as a known difference through a skip predicate plus its own test (the `e`/`E` precedent: predicate at `input_oracle_test.go:289-293`, used at `:322-323`, own test in `export_test.go`). The copy of the old code is never edited. Because focus is part of the compared state, no existing key may move the focus (section 4.7).
- `tview` reads `[text]` as a color tag, in table cells too. The Docker path escapes log lines and sparklines (`updateLogsView`, `tview.Escape`) and nothing else; its table cells stay as they are. Every dynamic text the AWS code puts into a tview primitive (account names, IDs, ARNs, role names, log group names, status texts, error texts, the title suffix) goes through `tview.Escape`.
- `make field-fix` reorders struct fields: use keyed struct literals everywhere, tests included.
- Never mention AI, AI tools or AI co-authorship in code, comments, commits, PRs or project documentation. This document and the implementation plan describe the work and its gates without naming tools or models; the execution ledger lives outside the versioned tree.
- No commit and no push without an explicit order from the owner. Work stays in the working tree of `feat/aws-integration` until the owner says otherwise.

## 4. Decisions

Marked **(owner)** when the owner said it, **(call)** when it is a choice made here that the owner can veto. Section 7 lists the calls made after the independent read of this document.

### 4.1 SDK and credentials (call, approved in conversation)

- Use `github.com/aws/aws-sdk-go-v2` with these modules: `config`, `credentials` (for `ssocreds`), `feature/ec2/imds` (only for the option that disables it), `service/sts`, `service/sso`, `service/cloudwatchlogs`. Pin the newest versions published on the Go proxy (as of 2026-10-22: core v1.47.4, config v1.33.10, credentials v1.20.10, sts v1.51.5, sso v1.38.5, cloudwatchlogs v1.90.2). `go mod tidy` decides the indirect ones.
- Credentials come from `config.LoadDefaultConfig`. GoWatch never parses credentials itself and never stores them. The default chain resolves, in the SDK's order, environment variables, the shared config and credentials files (including SSO profiles, both legacy `sso_*` keys and `sso_session` sections), web identity, container and instance roles. Inside a profile the SDK takes static keys in the shared files before the profile's `sso_*` settings; on the owner's machine the `[default]` profile has both, so identity uses the exported temporary keys while they are valid, and SSO discovery (4.4) uses the profile's `sso_*` settings and the cached token regardless.
- The `http.Client` field of `AWSClientManager` goes away or becomes the SDK's HTTP client option; it must not stay as dead code.

### 4.2 Enablement and startup (call)

`aws.enabled` keeps its three values.

- `false`: nothing AWS runs; the view says "AWS integration is disabled (aws.enabled: false)".
- `true`: the poller starts and the SDK's full default chain is tried, instance metadata (IMDS) included. The README tells users on EC2 instances with an instance role to set this value.
- `auto` (default): the poller starts only when something signals an AWS setup, so a laptop with no AWS footprint never probes the network. Signals: any of `AWS_ACCESS_KEY_ID`, `AWS_PROFILE`, `AWS_REGION`, `AWS_DEFAULT_REGION`, `AWS_ENDPOINT_URL`, `AWS_WEB_IDENTITY_TOKEN_FILE`, `AWS_CONTAINER_CREDENTIALS_RELATIVE_URI`, `AWS_CONTAINER_CREDENTIALS_FULL_URI` set; or `aws.profile`, `aws.region`, `aws.endpoint_url`, `aws.accounts` or `aws.cloudwatch.follow` set in the config; or the shared config file (`AWS_CONFIG_FILE` or `~/.aws/config`) or shared credentials file (`AWS_SHARED_CREDENTIALS_FILE` or `~/.aws/credentials`) exists. Without a signal the view says "AWS not configured" and explains the two ways to turn it on (configure the AWS CLI, or set `aws.enabled: true`).
- In `auto` the instance metadata provider is disabled (`config.WithEC2IMDSClientEnableState(imds.ClientDisabled)`), so a weak signal such as a `~/.aws/config` with only a region never makes GoWatch probe `169.254.169.254` on every pass. Container credentials still work in `auto`: their environment variables are a signal and their provider is separate from IMDS.
- Nothing AWS happens on the main goroutine. `main` creates the poller and starts it; the first SDK config load, the first `GetCallerIdentity` and every later call run inside the poller's goroutine with a per-call timeout (10 s). The view shows "connecting" until the first result.
- The poller stops with the application (context cancellation plus a bounded `Stop`, like the OTLP receiver).

Tests for the detection must redirect `HOME`, `AWS_CONFIG_FILE` and `AWS_SHARED_CREDENTIALS_FILE` to a temporary directory and blank every `AWS_*` variable, so the host's `~/.aws` never leaks into a test.

### 4.3 Profile, region and endpoint resolution (call)

- Profile: `AWS_PROFILE` wins; otherwise `aws.profile` from the config (passed as `config.WithSharedConfigProfile`); otherwise the SDK default (`default`).
- Region: `AWS_REGION`, then `AWS_DEFAULT_REGION`, then `aws.region` from the config (passed as `config.WithRegion` only when the environment has none), then the region of the resolved profile, then `us-east-1`. This keeps the existing rule "environment beats config file, config file beats the fallback" and inserts the profile's region before the fallback, which is what makes the owner's setup work (their region lives in the profile). The title of the AWS view shows the region that was actually resolved.
- Endpoint: `AWS_ENDPOINT_URL` (honored by the SDK natively), otherwise `aws.endpoint_url` from the config (`config.WithBaseEndpoint`). Used for LocalStack-style setups; applies to the STS client (and to the CloudWatch Logs client of stage 3). The SSO portal and OIDC clients, GoWatch's own and the ones the SDK's SSO credential provider builds, ignore every endpoint override and always call AWS in the SSO region of the profile, so the access token of the login is never sent anywhere else (owner decision, 2026-10-25).
- The existing precedence tests in `internal/aws/aws_test.go` are updated to these rules, not deleted. `AWS_DEFAULT_REGION` becomes an enabling signal (4.2), which changes one of them on purpose.

### 4.4 Accounts (owner: show the linked accounts; call: how)

- Identity: `sts:GetCallerIdentity` on the credentials the SDK resolves for the profile gives the account ID, ARN and user ID. This account is always monitored. It appears as a row of type `Account`, status `monitored`, name taken from the SSO account list when available, otherwise the account ID.
- SSO detection: `config.LoadSharedConfigProfile` on the resolved profile reports `SSOStartURL`, `SSORegion`, `SSOAccountID`, `SSORoleName` and, for `sso_session` profiles, an `SSOSession`. Either form makes the profile SSO-based for discovery, even when identity was resolved from exported static keys.
- Token: for `sso_session` profiles the bearer token comes from the SDK's SSO token provider (`ssocreds.SSOTokenProvider.RetrieveBearerToken`), which refreshes an expired access token through `sso-oidc:CreateToken` and rewrites its own cache file, exactly as the AWS CLI does. For legacy profiles (`sso_start_url` on the profile, no `sso_session`, as on the owner's machine) there is no refresh: GoWatch reads `accessToken` and `expiresAt` from the one cache file the SDK's path function names (`ssocreds.StandardCachedTokenFilepath`, keyed by the `sso_session` name or, for legacy profiles, by the start URL) and treats a missing, malformed or expired one as `SSO token expired` with the hint `run aws sso login`. The token stays in memory only, is never logged and is never written by GoWatch's own code.
- Discovery: `sso:ListAccounts` (paginated, cap 100 accounts) and `sso:ListAccountRoles` for each account (cap 20 roles) in the `sso_region`. Each account becomes a row: name, ID, roles, status `monitored` or `available`. Discovery runs at the discovery interval, so GoWatch recovers after an `aws sso login` without a restart.
- Monitoring more accounts: `aws.accounts` lists entries `"<account id>"` or `"<account id>/<role name>"`. For each entry found in the SSO list, GoWatch mints credentials with the SDK's `ssocreds` provider (`GetRoleCredentials` through the cached token) for that account and role, builds a CloudWatch Logs client with them in the resolved region, and marks the account `monitored`. Without a role, the role of the default profile (`sso_role_name`) is used when that account lists it; otherwise the first role listed, noted in the status. An entry not present in the SSO list is shown with status `error: not accessible with this login`. Without SSO, entries are shown as `requires SSO`.
- Account rows never show the account's e-mail address (PII without a purpose here).
- Stage boundary: stage 1 has no SSO code at all. It resolves identity and classifies the SDK's errors into hints (4.10). Reading the token, listing accounts and minting credentials are stage 2.

### 4.5 CloudWatch Logs (owner: CloudWatch logs first; call: how)

- Listing: for every monitored account, `DescribeLogGroups` in the resolved region (paginated), optional `aws.cloudwatch.log_group_prefix`, cap `aws.cloudwatch.max_log_groups` (default 100). Each group is a row of type `LogGroup`: name, account, status (`following`, `following (lagging)`, `not followed (cap)` or `-`), metrics summary with stored size and retention, last updated = time of the poll.
- Following: `aws.cloudwatch.follow` lists exact log group names or prefixes ending in `*`, matched against the listed groups of every monitored account. At most `aws.cloudwatch.max_followed` groups (default 10, accepted range 1 to 50) are tailed across all accounts; matches beyond the cap are listed with status `not followed (cap)`. A followed exact name that is not in the listing (prefix cap, or a group created later) is still requested directly by name.
- Tailing, per followed group and per poll:
  - The group keeps a cursor: the timestamp of the newest event accepted so far, in milliseconds.
  - The first poll asks from `now - aws.cloudwatch.lookback` (default 5 m). Every later poll asks from `cursor - 60 s`. That fixed 60 s overlap catches events ingested late with a timestamp older than the cursor; events that arrive later than that are lost, and the README says so.
  - The request is `FilterLogEvents` with `startTime` as above and no `endTime`; `nextToken` is followed until the service returns none or until `aws.cloudwatch.max_events_per_poll` events (default 1000, range 1 to 10000) have been received in this poll. Each page is one call.
  - Events are de-duplicated by event ID against the IDs accepted in the overlap window (IDs older than `cursor - 60 s` are pruned). Accepted events move the cursor forward; `FilterLogEvents` returns events in timestamp order, so the cursor ends at the newest event of the poll.
  - If the poll stopped because of the cap, the group is marked `following (lagging)`: the next poll continues from the cursor, nothing is skipped, and the group catches up as soon as its rate drops below `max_events_per_poll / refresh_interval`. The status returns to `following` on the first poll that ends without hitting the cap.
  - Per group, the last `aws.cloudwatch.buffer_lines` lines (default 200) are kept in a ring buffer; a group that stops being followed is dropped from the buffer on the next poll.
- The line shown is the event message as-is (CloudWatch events usually carry their own timestamp, like Lambda's `<time>\t<request id>\tINFO\t...`), with the Docker treatment applied: level from `docker.ParseLogLevel`, trace context through the correlator (`internal/aws` cannot import `internal/docker`, so the poller takes an optional `OnLine(service, line string)` hook that `main` wires to the correlator, through a getter `internal/docker` exposes), and escaping where the Docker path escapes (`updateLogsView` escapes the line).
- Service name of a line: the log group name, shortened for display by the builder's choice (for example dropping a leading `/aws/`), with the full name in the table. The export file keeps `service` and adds optional `account` and `log_group` fields (omitted for Docker lines) so the format stays backward compatible.
- Data flow into the panel. `docker.Containers` gains a cloud log source list (a source = account, group, display name, lines). `WatchContainers` fills it from the AWS manager's snapshot on every Docker refresh (a copy, no sharing of the ring buffer) and appends those lines to `FlatLogs` after the Docker lines, keeping lines grouped by source like containers. `filter.FilterContainers` rebuilds `FlatLogs` whenever a filter is active and today only walks `containers.C`, so it must apply the same rules to cloud sources: a source whose display name, full group name or account ID contains the search text is listed and contributes all of its lines; any other source contributes the lines that contain the text; the minimum log level applies to every line.
- Colors in the logs panel follow the rule containers have today (`getServiceColor`): a listed source takes the palette color of its position in the filtered list, and a source that matched only by text takes the color of its position in the unfiltered order (`serviceOrder`). The container behavior is pinned by `export_test.go:392-397` and must stay as it is; cloud sources join both lists after the containers.
- Quotas and cost: CloudWatch Logs has low per-account, per-region TPS quotas on `FilterLogEvents` and `DescribeLogGroups`. The defaults (10 followed groups, 15 s refresh, one `DescribeLogGroups` sweep per discovery interval) stay far below them. The README states that GoWatch makes about one `FilterLogEvents` call per followed group per refresh interval, more when a group is lagging, and how to lower it.

### 4.6 The AWS view (call)

- Table columns stay the six that exist: Resource Name, Type, Region, Status, Metrics Summary, Last Updated. Row types in this delivery: `Account` and `LogGroup`, accounts first, then groups grouped by account.
- Status colors keep the rule the old rows had: `StatusOK` for healthy statuses (`monitored`, `connected`, `following`), `StatusWarn` for the others (`available`, `not followed (cap)`, `following (lagging)`, `requires SSO`, `connecting`), and `StatusError` for statuses that start with `error`. In `legacy_look_test.go` the subtest `aws resources` is rewritten to feed two rows of the new model (one healthy, one warning) and to assert the same literal colors it asserts today; the rest of the file is untouched.
- The title keeps the prefix ` AWS Cloud Resources (<region>` (an interaction test checks the text `AWS Cloud Resources`) and gains a status after the region: `not configured`, `disabled`, `connecting`, `connected: <account id>` or `error: <short reason>`; a problem with the SSO discovery follows a connected status as `; <short reason>`. Hints (`run aws sso login`) go into the Status column of the affected rows, or into the single explanatory row when there is no account row; the title shows the short form of a problem (its text up to the first colon) and never a hint, and appends the discovery problem only to a connected status, so that every fixed title fits in 80 columns with the common region names (owner decision, 2026-10-25). Texts pass through `tview.Escape`.
- When the integration is off or not configured there is exactly one muted row explaining it, never a bare empty table. (The legacy subtest renders rows with the integration off; it exercises the row renderer, which draws whatever rows the state carries.)
- The Logs panel, the System Resources panel and the help bar are unchanged in this delivery (the help page lists the two new actions of stage 4).

### 4.7 Interactive follow and monitor (owner: "ou apertando Enter na linha"; call: how)

Last stage. Two new actions, both registered as known differences in `input_oracle_test.go` the way `e`/`E` were, with their own tests:

- `focus_next` (group Navigation, default key `Tab`): moves the focus between the services table and the logs panel. In the AWS view the table is row-selectable, so once it has the focus the arrows and PgUp/PgDn move the selection; in the Docker view the table keeps its current non-selectable look. With the logs panel hidden the key does nothing.
- `toggle_follow` (group Views, default key `Enter`): with the AWS table focused, toggles following for the selected log group and monitoring for the selected account (SSO only); anywhere else `Enter` keeps its current behavior (the search field handles its own `Enter` first, as today). The Status column reflects the change on the next poll, and the change lives for the session only (nothing is written to the config file).

No existing key changes behavior or focus: `a`/`A` and `d`/`D` switch the view exactly as today, and `Dashboard.mainPanel()` keeps deciding where `Esc`, the search field and the modals return the focus. The help page, the README table and `DefaultYAML()` list both actions; every help row fits 80 columns.

### 4.8 Architecture (call)

- `internal/aws` (rewritten around the existing manager):
  - `client.go`: `AWSClientManager` keeps the state snapshot and the options; `GetState()` returns a snapshot that shares nothing with the manager (today it copies only the top-level slice and shares each row's `Metrics` map; the new copy is deep for every slice and map). The state carries: resolved region, profile, endpoint, enabled flag with reason, connected flag, last error and hint, updated-at, identity, accounts, log groups, and the cloud log sources (the ring buffers' current lines). Fixed-data `Resources` disappear in favor of typed rows; the UI builds table rows from the typed data.
  - `sdk.go` (name free): builds the `aws.Config` and the service clients; defines the narrow interfaces the collectors use (`GetCallerIdentity`; `ListAccounts`, `ListAccountRoles`, `GetRoleCredentials`, the bearer token retrieval; `DescribeLogGroups`, `FilterLogEvents`) so tests use fakes and never the real clients.
  - `accounts.go`: identity and SSO discovery, per-account client minting.
  - `cloudwatch.go`: log group listing, follow matching, tailing, ring buffers.
  - `poller.go`: the goroutine. `Start(ctx)`, `Stop(ctx)`; a ticker at `aws.refresh_interval` for events; a discovery pass (identity, accounts, groups) every `aws.discovery_interval`; per-call timeouts; errors recorded in the state and retried on the next pass; interactive toggles received through methods on the manager (session-only overrides merged with the config lists).
  - `lambda.go`, `xray.go`, `cloudformation.go` and their fixed data are deleted together with their tests (section 4.15). The real ones return in P4.2 to P4.4.
  - `Options` gains the new settings (4.9) and an `IsZero()` method. `docker.Configure` currently compares `o.AWS != (aws.Options{})`, which stops compiling once `Options` holds slices; it switches to `IsZero()` and keeps the contract "a zero `Options` changes nothing".
- `internal/docker/collector.go`: `Configure` builds the manager from `aws.Options` when they are not zero (the package-level default manager is built from the environment only, as today); `WatchContainers` copies the state and the cloud log sources into `Containers` and appends their lines to `FlatLogs`; a getter exposes the correlator entry point `main` wires into the poller's `OnLine` hook.
- `internal/filter/filter.go`: cloud log sources filtered as in 4.5; `filtered.AWS` copied as today.
- `internal/ui`: `updateServicesTable` renders accounts and groups and the status in the title; `updateLogsView` colors cloud sources; stage 4 adds selection and the two actions.
- `cmd/gowatch/main.go`: builds the manager through `docker.Configure`, starts the poller when enabled with the correlator hook, stops it on exit.

### 4.9 Configuration (call)

New keys under `aws`, all optional, zero value = built-in default, strict decoding as today:

```yaml
aws:
  enabled: auto            # auto | true | false (exists)
  region: ""               # exists; see 4.3 for precedence
  endpoint_url: ""         # exists; refused from ./gowatch.yaml
  profile: ""              # shared config profile; AWS_PROFILE wins
  refresh_interval: 15s    # events poll; 5s to 10m
  discovery_interval: 60s  # identity, accounts and log groups; 15s to 1h
  accounts: []             # "<account id>" or "<account id>/<role>", SSO only
  cloudwatch:
    log_group_prefix: ""   # DescribeLogGroups prefix; empty lists every group up to the cap
    max_log_groups: 100    # 1 to 1000
    follow: []             # exact names or prefixes ending in *
    max_followed: 10       # 1 to 50
    lookback: 5m           # first poll window; 1m to 24h
    max_events_per_poll: 1000  # events accepted per group per poll; 1 to 10000
    buffer_lines: 200      # lines kept per followed group; 10 to 5000
```

- Validation: ranges above, account entries matching `^\d{12}(/[\w+=,.@-]+)?$`, non-empty follow entries, `*` only as the last character. Errors use the existing style ("aws.cloudwatch.max_followed must be between 1 and 50, got 80").
- Untrusted layer: `aws.endpoint_url` stays refused from `./gowatch.yaml`. `aws.profile`, `aws.accounts` and `aws.cloudwatch.follow` are honored from every layer: they only choose among the user's own credentials and read-only data shown locally, they cannot redirect traffic or expose anything. This reasoning goes into the comment of `untrustedSettings` and into the README.
- `DefaultYAML()` documents every key with its default and range; the existing printed-config tests cover them.
- `aws.Options` carries the new values; a zero `Options` keeps today's behavior.

### 4.10 Errors and hints (call)

The state records the last error per concern (connection, discovery, per account, per group) as a short, user-facing text plus a hint when one is known. Stage 1 classifies the SDK's errors on the identity call; later stages reuse the same classification:

- No credential provider found: `no AWS credentials found` + `configure the AWS CLI or set aws.enabled: false`.
- The SDK's SSO token errors (expired or missing cached token, invalid SSO session): `SSO token expired` + `run aws sso login`.
- STS rejects static credentials (`ExpiredToken`, `InvalidClientTokenId`, `UnrecognizedClientException`, `SignatureDoesNotMatch`): `credentials expired or invalid` + `log in again and re-export the keys` (shortened on 2026-10-25 so the row fits in 80 columns).
- Access denied on a call: `access denied: <operation>` (the ARN in the account row helps the user ask for the permission).
- Throttling: `throttled by AWS` and that pass is skipped (no retry storm).
- Timeouts and network errors: `AWS unreachable: <short cause>`.

Raw SDK errors are trimmed to one line and never include request bodies or tokens.

### 4.11 Security (call)

- Read-only AWS operations only: `sts:GetCallerIdentity`, `sso:ListAccounts`, `sso:ListAccountRoles`, `sso:GetRoleCredentials`, `logs:DescribeLogGroups`, `logs:FilterLogEvents`, plus `sso-oidc:CreateToken` when the SDK refreshes the token of an `sso_session` profile (and the equivalent call of the `aws login` session provider, if the pinned SDK supports it). The README lists them as the permissions and calls involved.
- GoWatch's own code never writes under `~/.aws`. The SDK's token provider may refresh and rewrite its own token cache file for `sso_session` profiles, which is the AWS CLI's behavior too; the README says so.
- The SSO token file read by GoWatch (legacy profiles) is the single path the SDK computes; GoWatch never lists or reads other cache files.
- No credential, token or account e-mail in state texts, logs, exports or tests. Test fixtures use obviously fake IDs (`123456789012`).
- `gosec` stays clean without blanket exclusions: file paths cleaned, integer conversions checked (G115), no `#nosec` without a one-line justification.

### 4.12 Testing (call)

- No test opens a network connection. Collectors and the poller are tested with fakes of the narrow interfaces; the real SDK clients are only constructed in the non-test wiring. Tests set `HOME`, `AWS_CONFIG_FILE`, `AWS_SHARED_CREDENTIALS_FILE` to temporary paths and blank the `AWS_*` variables.
- Coverage expected: enablement signals and the IMDS switch per mode; profile/region/endpoint precedence; error classification into hints; SSO detection and token file parsing (fresh, expired, missing, malformed); account discovery with pagination and caps; `aws.accounts` resolution (found, with role, without role, not accessible, non-SSO); log group listing with prefix and cap; follow matching (exact, prefix, cap, direct request of an unlisted group); tailing (first poll lookback, overlap and de-duplication, paging up to the cap, lagging status and recovery, cursor progression, buffer trimming, unfollow); poller lifecycle (start, stop, context cancel, no goroutine leak, errors recorded and retried, throttling skip); deep-copy of the snapshot under `-race`; `WatchContainers` merging cloud lines; `FilterContainers` rules for cloud sources; colors of cloud sources with and without filters; UI rendering of accounts, groups, status title and the single explanatory row on a `tcell.SimulationScreen`; stage 4 focus, selection and toggles end to end with `startLive()`; config defaults, validation ranges, `DefaultYAML()` coverage and the untrusted layer.
- `make check` (which is `make lint` plus `make test`), `make build` and `govulncheck ./...` pass at the end of every stage.

### 4.13 Documentation (call)

Each stage updates what it touches; the last stage sweeps:

- `README.md`: the AWS section loses the "Known limitation" paragraph and documents the credential chain, the SSO flow (legacy and `sso_session` profiles, the token refresh), every `aws.*` key, the permissions and calls involved, the cost bounds, the 60 s late-event window, and the two new keys.
- `internal/config/defaultyaml.go`: the `aws` section and the new actions.
- `.context/docs/project-roadmap.md`: Phase 4 status (foundation and P4.1 done or partial, with the gaps; P4.2 to P4.4 not started, no more "placeholder code").
- `.context/docs/project-overview.md`, `.context/docs/README.md`, `.context/docs/testing-strategy.md`, `.context/agents/feature-developer.md`, `.context/agents/bug-fixer.md`: the `internal/aws` descriptions.
- `.context/agents/active-memory.md` (in Portuguese, as the file is): what was delivered, validations run, technical notes, candidates for the next delivery.

### 4.14 Delivery stages and gates (owner directive)

The owner's directive for the process: a planning pass turns this document into a task plan; each stage is implemented, then independently validated against its acceptance criteria by a fresh reviewer who gets the stage specification and the diff (not the implementer's conclusions) and runs `make check`, `make build` and `govulncheck ./...` itself, returning APROVADO or REPROVADO with findings. Findings go back to the implementer, at most three rounds, then the owner is asked. After approval the owner sees the diff summary, the validator's report and the checks, and decides about the commit. Which tools or people fill those roles is recorded in the execution ledger, outside the versioned tree.

Stages, each leaving the application working and every gate green:

1. **Foundation and identity.** SDK modules in `go.mod`; `aws.Config` built in the poller with the resolution rules of 4.3; enablement and IMDS rule of 4.2; poller lifecycle; `GetCallerIdentity`; error classification of 4.10; the identity row and the status in the title; the single explanatory row when off; the fixed-data collectors removed; `Options.IsZero()`; config keys `profile`, `refresh_interval`, `discovery_interval` with validation and `DefaultYAML()`; precedence tests rewritten; the `aws resources` legacy subtest adapted; README and roadmap updated for this stage.
   Acceptance: without AWS configuration no call is made and the view says so; with credentials the owner sees `connected: <account id>` and the account row (manual smoke by the owner, since CI has no credentials); credential errors are classified and shown with their hints (unit-tested with fake errors); startup time and the Docker loop unchanged; `input_oracle_test.go` untouched and passing; `legacy_look_test.go` changed only in the `aws resources` subtest.
2. **SSO accounts.** Discovery of 4.4, token handling for both profile forms, `aws.accounts`, per-account credentials, account rows with roles and statuses, hints.
   Acceptance: with the owner's SSO login the view lists the linked accounts, the identity account as `monitored`, the others as `available`; `aws.accounts` entries become `monitored` or show the right error; without SSO only the identity row appears and entries say `requires SSO`; an expired legacy token shows `SSO token expired` with the hint and recovers after `aws sso login` without a restart.
3. **CloudWatch Logs through the config.** Listing, `follow`, tailing with the algorithm of 4.5, ring buffers, cloud log sources into `Containers`, `FlatLogs`, `FilterContainers`, colors, export fields, correlator hook, bounds, README cost note, config keys of `cloudwatch`.
   Acceptance: followed groups' events appear in the Logs panel with the group as service, filtered by search and level, exported with `account` and `log_group`, correlated; the call budget per poll is proven by a fake-counting test; overlap de-duplication, lookback, lagging and recovery covered.
4. **Interactive follow and monitor.** Section 4.7, help page, README, `DefaultYAML()`, oracle known differences, live test.
   Acceptance: `Tab` moves the focus, the selection moves with the arrows, `Enter` toggles the state within one poll, visible in the Status column; every existing key behaves as before; help page fits 80 columns.
5. **Documentation sweep and memory.** Section 4.13 completed, roadmap "Current State" rewritten, active memory updated.

The planner may re-cut the stages, but every cut must keep each stage shippable on its own and keep stage 1 free of SSO and CloudWatch code, so the owner can commit the foundation early.

### 4.15 Removing the fixed-data collectors (call)

`internal/aws/lambda.go`, `xray.go`, `cloudformation.go` and the sample data in `cloudwatch.go` are deleted in stage 1. Keeping code that nothing calls and that returns invented resources misleads readers and the roadmap; the real collectors come with P4.2 to P4.4 behind the interfaces of 4.8. The roadmap entries for P4.2 to P4.4 change from "placeholder code exists" to "not started".

## 5. Left to the builder

- Exact struct layouts, file names inside `internal/aws`, and the YAML shape details not fixed above (within the constraints of 4.9).
- The short display name of a log group in the Logs panel and how long names are truncated in the table.
- The exact status texts beyond the ones listed in 4.6 and 4.10, and the title layout, as long as the prefix stays and the information of 4.6 is there.
- How focus and selection are drawn in the AWS table (tview's selection style from the theme), and whether `Tab` is also accepted while the alerts list is open.
- How the correlator hook is exposed from `internal/docker` to `main`.
- Test helper design (fake clients, counting fakes, temporary HOME helpers).
- Whether `discovery_interval` is expressed as a duration or as a multiple of the refresh interval, as long as the defaults and ranges of 4.9 hold.

## 6. Open questions

None. The owner approved the description in conversation on 2026-10-25; the choices marked (call) above, and the ones in section 7, are open to veto before stage 1 starts.

## 7. Calls made after the independent read (2026-10-25)

A fresh reader of the first version of this document compared it with the code and raised six questions and several contradictions. All were settled here as calls; none changes what the owner asked for.

1. **`legacy_look_test.go`**: its `aws resources` subtest feeds the old `AWSResource` rows and would stop compiling. Call: adapt that one subtest to the new row model, same literal colors, same healthy/warning rule; the rest of the file untouched (2, 4.6).
2. **Focus and the keyboard oracle**: giving the table the focus on `a` would change an existing key's recorded state. Call: no existing key moves the focus; a new `focus_next` action on `Tab` does, and `toggle_follow` on `Enter` acts on the selected row; both registered as known differences like `e`/`E` (4.7).
3. **Identity versus SSO token**: the SDK resolves the owner's identity from the exported static keys, not from the SSO token, and never reads the CLI's own cache. Call: stage 1 classifies SDK and STS errors into hints and has no SSO code; the token is read in stage 2; expired static keys get their own hint (4.1, 4.4, 4.10).
4. **Token refresh under `~/.aws`**: the SDK's token provider refreshes and rewrites the cache of `sso_session` profiles. Call: accept it, use the SDK provider for `sso_session` profiles and the plain file read for legacy ones, list `sso-oidc:CreateToken` among the calls, and say in the README that GoWatch's own code never writes under `~/.aws` (4.4, 4.11).
5. **Tail semantics**: "+1 ms" left nothing to de-duplicate and a busy group would fall behind forever. Call: fixed 60 s overlap with de-duplication by event ID, paging up to `max_events_per_poll` (now 1000), a visible `following (lagging)` status, no skipping (4.5).
6. **Instance metadata in `auto`**: a weak signal would make laptops probe `169.254.169.254`. Call: `auto` disables IMDS, `true` keeps it (4.2).

Contradictions fixed in the text: `GetState()` is not a deep copy today (4.8); log colors follow the filtered position for listed services and `serviceOrder` only for text-only matches (4.5); `docker.Configure` compares `aws.Options` with `!=`, which needs `IsZero()` once slices exist (4.8); the Docker path escapes log lines and sparklines only, so the AWS cells must escape their own texts (3); process roles are described without naming tools, to respect the project's documentation rule (3, 4.14).
