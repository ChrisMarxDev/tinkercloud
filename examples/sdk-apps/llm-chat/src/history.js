// Conversation context is deliberately tab-local. Keep complete pairs only:
// a failed or cancelled request must never leave an orphaned user turn that
// would make the next provider request invalid.
const MAX_MESSAGES = 10;

export function beginTurn(history, content) {
  // Reserve room for the assistant reply. History is assembled solely by this
  // module, so retaining an even number preserves user/assistant pairs.
  const retained = history.slice(-(MAX_MESSAGES - 2));
  return [...retained, { role: "user", content }];
}

export function completeTurn(history, content) {
  return [...history, { role: "assistant", content }].slice(-MAX_MESSAGES);
}
