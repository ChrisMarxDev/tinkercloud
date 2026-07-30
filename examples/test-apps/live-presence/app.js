import { tinker } from "@tinkercloud/sdk";
const status = document.querySelector("#status");
const channel = tinker.live.channel("presence");
channel.on("changed", async () => { const state = await tinker.kv.get("presence/current"); status.textContent = JSON.stringify(state?.value ?? {}); });
await channel.connect();
// Events can be lost. The initial and reconnect recovery path always reads KV.
const current = await tinker.kv.get("presence/current"); status.textContent = JSON.stringify(current?.value ?? {});
