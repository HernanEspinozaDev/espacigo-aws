import http from "node:http";
const port = Number(process.env.COMMERCIAL_CONTRACT_FIXTURE_PORT ?? 18446);
if (!Number.isInteger(port) || port < 1024 || port > 65535)
  throw new Error("Invalid fixture port");
const server = http.createServer((request, response) => {
  response.setHeader("Content-Type", "application/json");
  response.setHeader("Cache-Control", "no-store");
  if (request.url === "/healthz" || request.url === "/health/ready") {
    response.end(JSON.stringify({ status: "ok" }));
    return;
  }
  response.writeHead(404);
  response.end(JSON.stringify({ error: { code: "not_found" } }));
});
server.listen(port, "127.0.0.1");
for (const signal of ["SIGINT", "SIGTERM"])
  process.on(signal, () => server.close(() => process.exit(0)));
