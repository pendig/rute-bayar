export const LANDING_CREATE_PAYMENT_LINES = [
  "$ rutebayar pay create --provider xendit \\",
  "  --method payment_link \\",
  "  --reference agent-run-1001 \\",
  "  --amount 25000",
  "",
  "payment_url: https://checkout.example/...",
  "status: pending",
];

export const LANDING_WEBHOOK_LINES = [
  "$ rutebayar webhook serve --addr :8080",
  "listening on /webhooks/xendit",
  "listening on /webhooks/midtrans",
  "forwarding enabled: orders-api",
];

export const LANDING_AGENT_STATUS_LINES = [
  "$ rutebayar pay status --provider xendit --reference agent-run-1001",
  "reference: agent-run-1001",
  "provider: xendit",
  "status: paid",
  "",
  "$ rutebayar reconcile --provider xendit --reference agent-run-1001",
  "local status is in sync",
];

export const LANDING_QUICK_INSTALL_LINES = [
  "$ brew tap pendig/tap",
  "$ brew install rutebayar",
  "",
  "$ curl -fSL -o rutebayar https://github.com/pendig/rute-bayar/releases/latest/download/rutebayar-linux-amd64",
  "$ chmod +x rutebayar",
  "$ ./rutebayar --version",
  "",
  "$ rutebayar provider list",
  "$ rutebayar onboard xendit --environment sandbox",
];
