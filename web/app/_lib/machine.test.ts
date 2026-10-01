import { test, expect } from "bun:test";
import { machinePercent, machineRatio, machineSize } from "./machine";

test("los megabytes van sin decimales y los gigas con uno", () => {
  expect(machineSize(29 * 1000 * 1000)).toBe("29 MB");
  expect(machineSize(1000 * 1000 * 1000)).toBe("1 GB");
  expect(machineSize(1.6 * 1000 * 1000 * 1000)).toBe("1,6 GB");
});

test("un uso se muestra contra su tope", () => {
  expect(machineRatio(1.9 * 1000 * 1000 * 1000, 32 * 1000 * 1000 * 1000)).toBe("1,9 GB / 32 GB");
  expect(machineRatio(29 * 1000 * 1000, 32 * 1000 * 1000 * 1000)).toBe("29 MB / 32 GB");
});

test("el porcentaje se redondea", () => {
  expect(machinePercent(2.3 * 1000 * 1000 * 1000, 4.8 * 1000 * 1000 * 1000)).toBe("48%");
  expect(machinePercent(0, 32 * 1000 * 1000 * 1000)).toBe("0%");
  expect(machinePercent(4.8 * 1000 * 1000 * 1000, 4.8 * 1000 * 1000 * 1000)).toBe("100%");
});
