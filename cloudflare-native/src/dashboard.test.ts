import { describe, expect, it } from "vitest";
import { dashboardResponse } from "./dashboard";

describe("dashboardResponse", () => {
  it("serves the authenticated jobs dashboard with defensive headers", async () => {
    const response = dashboardResponse(false);

    expect(response.headers.get("Content-Type")).toBe("text/html; charset=utf-8");
    expect(response.headers.get("Content-Security-Policy")).toContain("connect-src 'self'");
    expect(await response.text()).toContain("Freegent runs");
  });

  it("loads jobs without a token for a public test dashboard", async () => {
    const html = await dashboardResponse(true).text();

    expect(html).toContain("const publicDashboard = true");
    expect(html).toContain("public-dashboard-true");
    expect(html).toContain("Loading jobs");
    expect(html).toContain("if ((!publicDashboard && !state.token) || state.refreshing) return");
  });
});
