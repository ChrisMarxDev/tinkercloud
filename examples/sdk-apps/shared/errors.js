// @ts-check
import {
  TinyCapabilityUnavailableError,
  TinyError,
  TinyNotAuthenticatedError,
  TinyNotAuthorizedError,
  TinyQuotaExceededError,
  TinyRateLimitedError,
  TinyTemporarilyUnavailableError,
  TinyValidationError,
  TinyVersionConflictError,
  TinyVersionIncompatibleError,
} from "@tinyhost/sdk";

/**
 * Convert SDK errors into safe, useful application copy.
 * @param {unknown} error
 */
export function describeTinyError(error) {
  let title = "Something went wrong";
  let message = "Try the action again.";
  let retryable = true;

  if (error instanceof TinyNotAuthenticatedError) {
    title = "Sign-in expired";
    message = "Reload the page and sign in to this app again.";
  } else if (error instanceof TinyNotAuthorizedError) {
    title = "Access changed";
    message = "Your current session no longer has access to this app.";
    retryable = false;
  } else if (error instanceof TinyCapabilityUnavailableError) {
    title = "Capability unavailable";
    message = "This app was deployed without the capability it needs.";
    retryable = false;
  } else if (error instanceof TinyValidationError) {
    title = "That value cannot be saved";
    message = "Check the entry and keep it within the app limits.";
    retryable = false;
  } else if (error instanceof TinyVersionConflictError) {
    title = "Someone changed this first";
    message = "The latest state has been loaded. Review it and try again.";
  } else if (error instanceof TinyQuotaExceededError) {
    title = "App storage is full";
    message = "Remove something before adding more.";
    retryable = false;
  } else if (error instanceof TinyRateLimitedError) {
    title = "A little too fast";
    message = "Wait a moment, then try again.";
  } else if (error instanceof TinyTemporarilyUnavailableError) {
    title = "TinyHost is unavailable";
    message = "Your current view is still here. Reconnect and retry shortly.";
  } else if (error instanceof TinyVersionIncompatibleError) {
    title = "SDK update needed";
    message = "Rebuild this app with a compatible @tinyhost/sdk version.";
    retryable = false;
  } else if (error instanceof TinyError) {
    message = error.message;
  } else if (error instanceof DOMException && error.name === "AbortError") {
    title = "Request cancelled";
    message = "No changes were made.";
  } else if (error instanceof Error) {
    message = error.message;
  }

  return {
    title,
    message,
    retryable,
    requestId: error instanceof TinyError ? error.requestId : undefined,
  };
}
