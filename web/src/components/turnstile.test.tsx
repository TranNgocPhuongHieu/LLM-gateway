// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Turnstile } from "./turnstile";

afterEach(() => {
  cleanup();
  delete window.turnstile;
  document.getElementById("cf-turnstile")?.remove();
});

it("renders an unmodified dark flexible widget and cleans up without rerendering on callback changes", () => {
  const renderWidget = vi.fn(() => "widget-id");
  const remove = vi.fn();
  window.turnstile = { render: renderWidget, remove };
  const view = render(<Turnstile siteKey="configured-site-key" onVerify={vi.fn()} />);
  expect(renderWidget).toHaveBeenCalledWith(
    expect.any(HTMLElement),
    expect.objectContaining({
      sitekey: "configured-site-key",
      theme: "dark",
      size: "flexible",
    }),
  );
  const verify = vi.fn();
  view.rerender(<Turnstile siteKey="configured-site-key" onVerify={verify} />);
  expect(renderWidget).toHaveBeenCalledTimes(1);
  const options = renderWidget.mock.calls[0] as unknown as [
    HTMLElement,
    { callback: (token: string) => void },
  ];
  options[1].callback("verified-token");
  expect(verify).toHaveBeenCalledWith("verified-token");
  view.unmount();
  expect(remove).toHaveBeenCalledWith("widget-id");
});

it("renders when an existing script finishes loading after the modal reopens", () => {
  const script = document.createElement("script");
  script.id = "cf-turnstile";
  document.head.appendChild(script);
  render(<Turnstile siteKey="configured-site-key" onVerify={vi.fn()} />);
  const renderWidget = vi.fn(() => "widget-id");
  window.turnstile = { render: renderWidget, remove: vi.fn() };
  script.dispatchEvent(new Event("load"));
  expect(renderWidget).toHaveBeenCalledTimes(1);
});
