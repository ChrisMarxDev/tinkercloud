import { tiny } from "@tinyhost/sdk";
const output = document.querySelector("#state");
async function refresh() { const current = await tiny.kv.get("counter"); output.textContent = JSON.stringify(current?.value ?? { count: 0 }); }
await tiny.user.current(); await refresh();
const updates = tiny.live.channel("counter"); updates.on("changed", refresh); await updates.connect();
