import { createRoot } from "react-dom/client";
import { act } from "react";
import { describe, expect, it, vi } from "vitest";
import { githubAppInstallUrl, SignInDialog, signedInWithGitHub } from "./SignIn";
import { api } from "../api";
import { t } from "../i18n";

vi.mock("../api", () => ({
  api: {
    getWelcome: () => Promise.resolve({ githubSignIn: true }),
    startGitHubSignIn: () => Promise.resolve({ userCode: "ABCD-1234", verificationUri: "https://github.com/login/device", expiresIn: 900, interval: 5 }),
    waitGitHubSignIn: () => Promise.resolve({ host: "github.com", signedIn: true, kind: "oauth" }),
    cancelSignIn: vi.fn(),
    openExternal: vi.fn(),
    getAuthStatus: vi.fn(),
  },
}));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

function button(label: string): HTMLButtonElement {
  const match = [...document.querySelectorAll("button")].find((element) => element.textContent?.includes(label));
  if (!match) throw new Error(`no button labelled ${label}`);
  return match;
}

describe("signing in with GitHub", () => {
  it("asks which repositories Pageroom may use before continuing", async () => {
    const onDone = vi.fn();
    const host = document.createElement("div");
    document.body.append(host);
    await act(async () => createRoot(host).render(<SignInDialog remote="https://github.com/a/b" onClose={() => {}} onDone={onDone} />));

    expect(document.body.textContent).toContain(t.signIn.selectRepositoriesBody);
    expect(onDone).not.toHaveBeenCalled();

    await act(async () => button(t.signIn.selectRepositories).click());
    expect(api.openExternal).toHaveBeenCalledWith(githubAppInstallUrl);

    await act(async () => button(t.common.continue).click());
    expect(onDone).toHaveBeenCalledOnce();
  });
});

describe("signedInWithGitHub", () => {
  it.each([
    [{ host: "github.com", signedIn: true, kind: "oauth" }, true],
    [{ host: "github.com", signedIn: true, kind: "token" }, false],
    [{ host: "github.com", signedIn: false }, false],
  ])("is %j → %s", async (status, expected) => {
    vi.mocked(api.getAuthStatus).mockResolvedValue(status);
    expect(await signedInWithGitHub("https://github.com/a/b")).toBe(expected);
  });

  it("is false when the status can't be read", async () => {
    vi.mocked(api.getAuthStatus).mockRejectedValue(new Error("no site"));
    expect(await signedInWithGitHub("")).toBe(false);
  });
});
