import { defineConfig } from "mygo-cli";

// Twill's window is drawn by mygo/ui from Go; there is no web frontend to
// build, so no devUrl, buildCommand or frontendDist.
export default defineConfig({
  name: "Twill",
  identifier: "com.example.twill",
  version: "0.1.0",
  out: "build",
});
