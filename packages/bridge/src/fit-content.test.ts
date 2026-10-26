import { describe, expect, test } from "bun:test";
import { fitRule, sizeReporter } from "./fit-content";

const none = { minWidth: 0, minHeight: 0, maxWidth: 0, maxHeight: 0 };

describe("fitRule", () => {
  test("lays out the root at its content's width, without specificity", () => {
    expect(fitRule(none, 0)).toBe(":where(:root){width:max-content}");
  });

  test("bounds the width and the minimum height, not the maximum height", () => {
    expect(fitRule({ minWidth: 100, minHeight: 40, maxWidth: 300, maxHeight: 200 }, 0)).toBe(
      ":where(:root){width:max-content;min-width:100px;max-width:300px;min-height:40px}",
    );
  });

  test("leaves a vertical scrollbar's room out of the width", () => {
    expect(fitRule({ ...none, minWidth: 100, maxWidth: 300 }, 15)).toBe(
      ":where(:root){width:max-content;min-width:85px;max-width:285px}",
    );
    expect(fitRule({ ...none, maxWidth: 10 }, 15)).toBe(":where(:root){width:max-content;max-width:0px}");
  });

  test("takes the fractional CSS pixels of a zoomed page", () => {
    expect(fitRule({ ...none, maxWidth: 133.5 }, 0)).toBe(":where(:root){width:max-content;max-width:133.5px}");
  });
});

describe("sizeReporter", () => {
  test("tells only of changes", () => {
    const told: [number, number][] = [];
    const report = sizeReporter((w, h) => told.push([w, h]));
    report(120, 80);
    report(120, 80);
    report(120.5, 80);
    report(120.5, 90);
    report(120.5, 90);
    report(0, 0);
    expect(told).toEqual([
      [120, 80],
      [120.5, 80],
      [120.5, 90],
      [0, 0],
    ]);
  });
});
