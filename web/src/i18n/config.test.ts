import { createInstance, type BackendModule } from "i18next";
import { describe, expect, it } from "vitest";

import { localeBackend, resources } from "./config";
import { convertDetectedLanguage } from "./languages";

describe("on-demand interface languages", () => {
  it("bundles English fallback only", () => {
    expect(Object.keys(resources)).toEqual(["en"]);
  });

  it.each(["vi", "en"])("loads %s and keeps translated/interpolated keys", async (language) => {
    const instance = createInstance().use(localeBackend);
    await instance.init({
      resources: structuredClone(resources),
      partialBundledLanguages: true,
      lng: language,
      fallbackLng: "en",
      supportedLngs: ["en", "vi"],
      load: "currentOnly",
      nsSeparator: false,
      interpolation: { escapeValue: false },
    });
    expect(instance.resolvedLanguage).toBe(language);
    expect(instance.t("Price Management")).toBe(language === "vi" ? "Quản lý giá" : "Price Management");
    for (const [key, vietnamese] of [
      ["Quota Check", "Kiểm tra quota"],
      ["Selling Prices", "Giá bán"],
      ["Purchase Prices", "Giá nhập"],
      ["Bulk adjustment", "Điều chỉnh hàng loạt"],
      ["Request Logs", "Nhật ký yêu cầu"],
    ]) {
      expect(instance.t(key)).toBe(language === "vi" ? vietnamese : key);
    }
    expect(instance.t("Inherited from {{vendor}}", { vendor: "Fixture" })).toContain("Fixture");
    expect(instance.t("This key is not translated")).toBe("This key is not translated");
    await instance.changeLanguage(language === "vi" ? "en" : "vi");
    expect(instance.resolvedLanguage).toBe(language === "vi" ? "en" : "vi");
    await instance.changeLanguage("en");
    expect(instance.resolvedLanguage).toBe("en");
  });

  it("keeps browser language mapping and unsupported language fallback", async () => {
    expect(convertDetectedLanguage("zh-CN")).toBe("en");
    expect(convertDetectedLanguage("zh-Hans")).toBe("en");
    const instance = createInstance().use(localeBackend);
    await instance.init({
      resources: structuredClone(resources),
      partialBundledLanguages: true,
      lng: "unsupported",
      fallbackLng: "en",
      supportedLngs: ["en", "vi"],
      nsSeparator: false,
    });
    expect(instance.resolvedLanguage).toBe("en");
  });

  it("reports load failures while keeping the bundled English fallback", async () => {
    const failingBackend: BackendModule = {
      ...localeBackend,
      read: (_language, _namespace, callback) =>
        callback(new Error("fixture chunk unavailable"), false),
    };
    const instance = createInstance().use(failingBackend);
    const failures: string[] = [];
    instance.on("failedLoading", (language) => failures.push(language));
    await instance.init({
      resources: structuredClone(resources),
      partialBundledLanguages: true,
      lng: "vi",
      fallbackLng: "en",
      supportedLngs: ["en", "vi"],
      nsSeparator: false,
    });
    expect(failures).toContain("vi");
    expect(instance.resolvedLanguage).toBe("en");
    expect(instance.t("Inherited from {{vendor}}", { vendor: "Fixture" })).toContain("Fixture");
  });
});
