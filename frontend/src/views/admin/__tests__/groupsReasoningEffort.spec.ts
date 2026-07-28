import { describe, expect, it } from "vitest";
import { nextTick } from "vue";
import { mount } from "@vue/test-utils";
import { createI18n } from "vue-i18n";

import ReasoningEffortPolicyFields from "@/components/admin/group/ReasoningEffortPolicyFields.vue";

import {
  createReasoningEffortMappingRow,
  normalizeReasoningEffortForPlatform,
  reasoningEffortMappingsToAPI,
  reasoningEffortMappingsToRows,
  reasoningEffortOptionsForPlatform,
  validateReasoningEffortMappings,
} from "../groupsReasoningEffort";

describe("groupsReasoningEffort", () => {
  it("provides the complete ordered OpenAI policy choices", () => {
    expect(
      reasoningEffortOptionsForPlatform("openai").map((option) => option.value),
    ).toEqual(["minimal", "low", "medium", "high", "xhigh", "max"]);

    for (const platform of ["anthropic", "gemini", "antigravity"] as const) {
      expect(reasoningEffortOptionsForPlatform(platform)).toEqual([]);
    }
  });

  it("hydrates canonical rows into an independent array", () => {
    const source = [{ from: " max ", to: " xhigh " }];
    const rows = reasoningEffortMappingsToRows(source, "openai");

    expect(reasoningEffortMappingsToAPI(rows)).toEqual([
      { from: "max", to: "xhigh" },
    ]);
    rows[0]!.to = "low";
    expect(source[0]!.to).toBe(" xhigh ");
  });

  it("drops stale custom values and clears non-OpenAI state", () => {
    expect(
      reasoningEffortMappingsToRows(
        [
          { from: "ultra", to: "high" },
          { from: "max", to: "xhigh" },
        ],
        "openai",
      ),
    ).toHaveLength(1);
    expect(normalizeReasoningEffortForPlatform("openai", " MAX ")).toBe(
      "max",
    );
    expect(normalizeReasoningEffortForPlatform("gemini", "max")).toBe("");
    expect(normalizeReasoningEffortForPlatform("openai", "none")).toBe("");
  });

  it("requires complete rows and rejects duplicate normalized sources", () => {
    const missingFrom = createReasoningEffortMappingRow({ to: "low" });
    const duplicateA = createReasoningEffortMappingRow({
      from: "MAX",
      to: "xhigh",
    });
    const duplicateB = createReasoningEffortMappingRow({
      from: " max ",
      to: "high",
    });
    const missingTo = createReasoningEffortMappingRow({ from: "low" });

    expect(
      validateReasoningEffortMappings([
        missingFrom,
        duplicateA,
        duplicateB,
        missingTo,
      ]),
    ).toEqual({
      [missingFrom.id]: { from: "fromRequired" },
      [duplicateA.id]: { from: "duplicateFrom" },
      [duplicateB.id]: { from: "duplicateFrom" },
      [missingTo.id]: { to: "toRequired" },
    });
  });

  it("adds rows and exposes inline validation in the mounted fields", async () => {
    const i18n = createI18n({
      legacy: false,
      locale: "en",
      messages: {
        en: {
          admin: {
            groups: {
              form: {
                maxReasoningEffort: "Maximum",
                maxReasoningEffortUnlimited: "Unlimited",
                maxReasoningEffortHint: "Hint",
                reasoningEffortMappings: "Mappings",
                addReasoningEffortMapping: "Add mapping",
                removeReasoningEffortMapping: "Remove mapping",
                reasoningEffortFrom: "From",
                reasoningEffortTo: "To",
                reasoningEffortFromPlaceholder: "Select from",
                reasoningEffortToPlaceholder: "Select to",
                fromRequired: "From required",
                toRequired: "To required",
              },
            },
          },
          common: {
            selectOption: "Select",
            searchPlaceholder: "Search",
            noOptionsFound: "No options",
          },
        },
      },
    });
    const wrapper = mount(ReasoningEffortPolicyFields, {
      props: {
        idPrefix: "test-policy",
        platform: "openai",
        maxEffort: "",
        mappings: [],
      },
      global: { plugins: [i18n] },
    });

    const add = wrapper
      .findAll("button")
      .find((button) =>
        button.text().includes("Add mapping") ||
        button.text().includes("addReasoningEffortMapping"),
      );
    expect(add).toBeDefined();
    await add!.trigger("click");
    const emittedRows = wrapper.emitted("update:mappings")?.[0]?.[0] as ReturnType<
      typeof createReasoningEffortMappingRow
    >[];
    expect(emittedRows).toHaveLength(1);

    await wrapper.setProps({ mappings: emittedRows });
    const exposed = wrapper.vm as unknown as { validate: () => boolean };
    expect(exposed.validate()).toBe(false);
    await nextTick();
    expect(wrapper.findAll('[role="alert"]')).toHaveLength(2);
  });
});
