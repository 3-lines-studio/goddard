const GB = 1000 * 1000 * 1000;
const MB = 1000 * 1000;

export function machineSize(bytes: number) {
  if (bytes >= GB) return decimal(bytes / GB) + " GB";
  return Math.round(bytes / MB) + " MB";
}

export function machineRatio(used: number, total: number) {
  return machineSize(used) + " / " + machineSize(total);
}

export function machinePercent(used: number, total: number) {
  return Math.round((used / total) * 100) + "%";
}

function decimal(value: number) {
  return value.toFixed(1).replace(".", ",").replace(",0", "");
}
