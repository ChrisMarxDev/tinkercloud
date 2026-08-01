// @ts-check
import {
  TinkerCapabilityUnavailableError,
  TinkerError,
  TinkerNotAuthenticatedError,
  TinkerNotAuthorizedError,
  TinkerQuotaExceededError,
  TinkerRateLimitedError,
  TinkerTemporarilyUnavailableError,
  TinkerValidationError,
  TinkerVersionConflictError,
  TinkerVersionIncompatibleError,
} from "@tinkercloud/sdk";

/**
 * Convert SDK errors into safe, useful application copy.
 * @param {unknown} error
 */
export function describeTinkerError(error) {
  let title = "Something went wrong";
  let message = "Try the action again.";
  let retryable = true;

  if (error instanceof TinkerNotAuthenticatedError) {
    title = "Sign-in expired";
    message = "Reload the page and sign in to this app again.";
  } else if (error instanceof TinkerNotAuthorizedError) {
    title = "Access changed";
    message = "Your current session no longer has access to this app.";
    retryable = false;
  } else if (error instanceof TinkerCapabilityUnavailableError) {
    title = "Capability unavailable";
    message = "This app was deployed without the capability it needs.";
    retryable = false;
  } else if (error instanceof TinkerValidationError) {
    title = "That value cannot be saved";
    message = "Check the entry and keep it within the app limits.";
    retryable = false;
  } else if (error instanceof TinkerVersionConflictError) {
    title = "Someone changed this first";
    message = "The latest state has been loaded. Review it and try again.";
  } else if (error instanceof TinkerQuotaExceededError) {
    title = "App storage is full";
    message = "Remove something before adding more.";
    retryable = false;
  } else if (error instanceof TinkerRateLimitedError) {
    title = "A little too fast";
    message = "Wait a moment, then try again.";
  } else if (error instanceof TinkerTemporarilyUnavailableError) {
    title = "Tinkercloud is unavailable";
    message = "Your current view is still here. Reconnect and retry shortly.";
  } else if (error instanceof TinkerVersionIncompatibleError) {
    title = "SDK update needed";
    message = "Rebuild this app with a compatible @tinkercloud/sdk version.";
    retryable = false;
  } else if (error instanceof TinkerError) {
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
    requestId: error instanceof TinkerError ? error.requestId : undefined,
  };
}
