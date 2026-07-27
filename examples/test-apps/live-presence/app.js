import { tiny } from "@tinyhost/sdk";
const status = document.querySelector("#status");
const channel = tiny.live.channel("presence");
channel.on("changed", async () => { const state = await tiny.kv.get("presence/current"); status.textContent = JSON.stringify(state?.value ?? {}); });
await channel.connect();
// Events can be lost. The initial and reconnect recovery path always reads KV.
const current = await tiny.kv.get("presence/current"); status.textContent = JSON.stringify(current?.value ?? {});
