function isRecord(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function validatePackResult(result, packageName) {
  if (!isRecord(result) || result.name !== packageName || !Array.isArray(result.files)) {
    throw new Error("npm pack JSON result must name the expected package and contain a files array");
  }

  if (!result.files.every((file) => isRecord(file) && typeof file.path === "string")) {
    throw new Error("npm pack JSON files must each contain a string path");
  }

  return result;
}

export function parseNpmPackJson(output, packageName) {
  let parsed;
  try {
    parsed = JSON.parse(output);
  } catch {
    throw new Error("npm pack --json did not produce valid JSON");
  }

  if (Array.isArray(parsed)) {
    if (parsed.length !== 1) {
      throw new Error("npm pack JSON array must contain exactly one package");
    }
    return validatePackResult(parsed[0], packageName);
  }

  if (!isRecord(parsed)) {
    throw new Error("npm pack JSON must be an array or a package-keyed object");
  }

  const names = Object.keys(parsed);
  if (names.length !== 1 || names[0] !== packageName) {
    throw new Error(`npm pack JSON must contain exactly the ${packageName} package`);
  }

  return validatePackResult(parsed[packageName], packageName);
}
