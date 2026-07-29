import { describe, expect, it } from "vitest";
import { dashboardResponse } from "./dashboard";

describe("dashboardResponse", () => {
  it("serves the authenticated jobs dashboard with defensive headers", async () => {
    const response = dashboardResponse();

    expect(response.headers.get("Content-Type")).toBe("text/html; charset=utf-8");
    expect(response.headers.get("Content-Security-Policy")).toContain("connect-src 'self'");
    expect(await response.text()).toContain("Freegent runs");
  });
});
