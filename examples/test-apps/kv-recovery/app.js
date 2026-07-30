import { tinker } from "@tinkercloud/sdk";
const output = document.querySelector("#state");
async function refresh() { const current = await tinker.kv.get("counter"); output.textContent = JSON.stringify(current?.value ?? { count: 0 }); }
await tinker.user.current(); await refresh();
const updates = tinker.live.channel("counter"); updates.on("changed", refresh); await updates.connect();
