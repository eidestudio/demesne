import { defineConfig } from "vitest/config";
import { fileURLToPath } from "node:url";

export default defineConfig({
  test: {
    include: ["test/**/*.test.ts"],

    testTimeout: 60_000,
    hookTimeout: 60_000,
  },
  resolve: {
    alias: {
      "@foir/demesne": fileURLToPath(new URL("../runtime/src/index.ts", import.meta.url)),
    },
  },
});
