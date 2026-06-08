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

export const LANDING_API_MODE_LINES = [
  "$ rutebayar api midtrans --operation status --path-param order_id=rb-001",
  "HTTP/2 200",
  "transaction_status: settle",
  "",
  "$ rutebayar api xendit --operation auth-balance",
  "HTTP/2 200",
  "message: Success",
];

export const LANDING_QUICK_INSTALL_LINES = [
  "$ curl -fsSL https://raw.githubusercontent.com/pendig/rute-bayar/main/scripts/install.sh | bash",
  "$ rutebayar version",
  "",
  "$ rutebayar provider list",
  "$ rutebayar onboard xendit --environment sandbox",
];
