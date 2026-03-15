export default {
  name: "deploq",
  description: "Lightweight webhook deploy tool for Docker Compose",
  navLinks: [
    { label: "Docs", href: "#introduction" },
    { label: "Configuration", href: "#configuration" },
    { label: "API", href: "#api" },
    { label: "GitHub", href: "https://github.com/us/deploq", external: true },
  ],
  sidebar: [
    {
      title: "Getting Started",
      children: [
        { title: "Introduction", slug: "introduction" },
        { title: "Installation", slug: "installation" },
        { title: "Quick Start", slug: "quick-start" },
      ]
    },
    {
      title: "Configuration",
      children: [
        { title: "Config File", slug: "configuration" },
        { title: "Event Filtering", slug: "event-filtering" },
        { title: "CI Status Checks", slug: "ci-status-checks" },
        { title: "Failure Hooks", slug: "failure-hooks" },
      ]
    },
    {
      title: "Webhook Setup",
      children: [
        { title: "GitHub Webhooks", slug: "github-webhooks" },
        { title: "Generic CI", slug: "generic-ci" },
      ]
    },
    {
      title: "Reference",
      children: [
        { title: "API Endpoints", slug: "api" },
        { title: "CLI Commands", slug: "cli" },
        { title: "Deploy Pipeline", slug: "pipeline" },
      ]
    },
    {
      title: "Operations",
      children: [
        { title: "Production Setup", slug: "production" },
        { title: "Changelog", slug: "changelog" },
      ]
    }
  ],
  defaultPage: "introduction",
  footer: {
    left: "Released under the MIT License",
    right: "Built with Vanilla HTML, CSS & JS",
  }
};
