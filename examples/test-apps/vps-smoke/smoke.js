const marker = "TINKERCLOUD_VPS_SMOKE_ASSET_MARKER";
document.querySelector("#asset").textContent = marker;

// The live VPS suite deploys a randomized equivalent of this fixture. It uses
// the same-origin SDK blob wire contract after Tinkercloud has authenticated the
// viewer; neither app identity nor any credential is configured here.
export async function exerciseBlobs(file) {
  const api = "/_tinker/api/v1";
  const capabilities = await fetch(`${api}/capabilities`, { credentials: "same-origin" }).then((r) => r.json());
  if (!capabilities.capabilities.some((capability) => capability.name === "blobs")) throw new Error("Blob capability unavailable.");
  const form = new FormData();
  form.append("file", file, file.name);
  const uploaded = await fetch(`${api}/blobs`, { method: "POST", body: form, credentials: "same-origin" }).then((r) => r.json());
  const listed = await fetch(`${api}/blobs`, { credentials: "same-origin" }).then((r) => r.json());
  const bytes = await fetch(`${api}/blobs/${encodeURIComponent(uploaded.id)}`, { credentials: "same-origin" }).then((r) => r.blob());
  await fetch(`${api}/blobs/${encodeURIComponent(uploaded.id)}`, { method: "DELETE", credentials: "same-origin" });
  return { capabilities, uploaded, listed, bytes };
}
