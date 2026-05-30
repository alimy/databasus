import type { Metadata } from "next";
import { CloudDashboardButton } from "../components/CloudDashboardButton";

export const metadata: Metadata = {
  title: "Databasus Cloud",
  robots: "index, follow",
  alternates: {
    canonical: "https://databasus.com/cloud",
  },
  icons: {
    icon: [
      { url: "/favicon.ico", type: "image/x-icon" },
      { url: "/favicon.svg", type: "image/svg+xml" },
    ],
    apple: "/favicon.svg",
    shortcut: "/favicon.ico",
  },
};

export default function Index() {
  const plans = [
    {
      name: "Starter",
      price: "$25",
      period: "/mo",
      from: false,
      featured: false,
      cta: { label: "Get started", href: "https://app.databasus.com" },
    },
    {
      name: "Pro",
      price: "$50",
      period: "/mo",
      from: false,
      featured: true,
      cta: { label: "Get started", href: "https://app.databasus.com" },
    },
    {
      name: "Business",
      price: "$100",
      period: "/mo",
      from: false,
      featured: false,
      cta: { label: "Get started", href: "https://app.databasus.com" },
    },
    {
      name: "Enterprise",
      price: "$250",
      period: "/mo",
      from: true,
      featured: false,
      cta: { label: "Go to Labs", href: "/labs" },
    },
  ];

  const rows: {
    label: string;
    tooltip?: string;
    values: (boolean | string)[];
  }[] = [
    {
      label: "Database size",
      values: ["Up to 50 GB", "Up to 250 GB", "Up to 1 TB", "1 TB+"],
    },
    {
      label: "Daily logical backups (DB <50GB)",
      values: [true, true, true, true],
    },
    {
      label: "Weekly full physical backups",
      tooltip:
        "Daily full backups are only efficient for small databases (under 50 GB). For larger databases they are slow and expensive, so Pro and above use weekly full physical backups plus daily / hourly incrementals.",
      values: [true, true, true, true],
    },
    {
      label: "Daily / hourly incremental backups",
      values: [true, true, true, true],
    },
    {
      label: "Retention",
      values: ["30 days", "30 days", "30 days", "Custom"],
    },
    {
      label: "Verified restore on daily basis",
      values: [true, true, true, true],
    },
    {
      label: "Email support",
      values: [true, true, true, true],
    },
    {
      label: "Priority email support",
      values: [false, true, true, true],
    },
    {
      label: "Dedicated support with SLA",
      values: [false, false, false, true],
    },
    {
      label: "RTO / RPO agreement",
      values: [false, false, false, true],
    },
    {
      label: "Managed in your environment",
      values: [false, false, false, true],
    },
  ];

  return (
    <div className="overflow-x-hidden">
      {/* HEADER */}
      <header className="fixed top-0 left-0 right-0 z-50 flex justify-center pt-3 md:pt-5 px-4 md:px-0">
        <div className="mx-auto w-full max-w-[1000px] 2xl:max-w-[1200px]">
          <nav className="relative flex items-center justify-between border backdrop-blur-md bg-[#0C0E13]/80 md:bg-[#0C0E13]/20 border-[#ffffff20] px-3 py-2 rounded-xl">
            <a href="/" className="flex items-center gap-2.5">
              <img
                src="/logo.svg"
                alt="Databasus logo"
                width={32}
                height={32}
                className="h-7 w-7 md:h-8 md:w-8"
                fetchPriority="high"
                loading="eager"
              />

              <span className="text-base md:text-lg font-semibold pl-1">
                Databasus
              </span>
            </a>

            {/* Desktop Navigation */}
            <div className="absolute left-1/2 -translate-x-1/2 hidden lg:flex items-center gap-3">
              <a
                href="/#how-to-use"
                target="_blank"
                className="py-2 hover:text-gray-300 transition-colors"
              >
                How to use
              </a>

              <a
                href="/#features"
                target="_blank"
                className="py-2 hover:text-gray-300 transition-colors"
              >
                Features
              </a>

              <a
                href="/installation"
                className="py-2 hover:text-gray-300 transition-colors"
              >
                Docs
              </a>

              <a
                href="#pricing"
                className="py-2 hover:text-gray-300 transition-colors"
              >
                Pricing
              </a>

              <a
                href="https://t.me/databasus_community"
                target="_blank"
                rel="noopener noreferrer"
                className="py-2 hover:text-gray-300 transition-colors"
              >
                Community
              </a>
            </div>

            <CloudDashboardButton variant="navbar" />
          </nav>
        </div>
      </header>

      {/* MAIN SECTION */}
      <main className="relative overflow-hidden pt-[60px] md:pt-[68px]">
        <div className="relative mx-auto w-full max-w-[1000px] 2xl:max-w-[1200px] px-4 md:px-6 lg:px-0 pt-12 md:pt-[100px] pb-12 md:pb-[100px]">
          {/* Background ellipse */}
          <div className="relative">
            <div className="absolute left-1/2 -translate-x-1/2 -translate-y-1/4 w-[400px] h-[400px] md:w-[900px] md:h-[900px] bg-[#155dfc]/4 top-0 rounded-full blur-3xl -z-10" />
          </div>

          {/* Content */}
          <div className="mb-8 md:mb-16 flex flex-col md:flex-row">
            <div className="w-full md:w-1/2 text-center md:text-left">
              <h1 className="text-xl sm:text-2xl 2xl:text-3xl leading-tight font-bold mb-4 md:max-w-[470px]">
                PostgreSQL backups with restore verification at optimized cost.
                So you are sure{" "}
                <span className="underline decoration-4 underline-offset-3 decoration-[#0d6efd]">
                  your data is safe
                </span>{" "}
              </h1>

              <p className="text-sm xl:text-lg text-gray-200 mb-6 max-w-[460px] mx-auto md:mx-0">
                Databasus cloud is backing up your database as primary or
                additional independent backups engine. Your backups{" "}
                <span className="underline decoration-2 underline-offset-2 decoration-[#0d6efd]">
                  are verified via automatic restore test
                </span>{" "} each day
                so you can be sure your backups are working as expected
              </p>

              <div className="max-w-[350px] mx-auto md:mx-0">
                <div className="flex flex-col gap-2">
                  <CloudDashboardButton variant="hero" />

                  <a
                    href="#pricing"
                    className="order-2 w-full inline-flex items-center justify-center gap-2 px-4 py-2 sm:px-12 sm:py-2.5 rounded-lg font-medium border border-[#0d6efd] border-2 text-[#0d6efd] hover:bg-[#0d6efd]/10 transition-colors cursor-pointer"
                  >
                    Pricing
                  </a>
                </div>

                <div className="mt-2 text-center text-sm max-w-[280px] mx-auto text-gray-500">
                  *you can always switch back to self-hosted, because we are{" "}
                  <a
                    href="https://github.com/databasus/databasus?tab=readme-ov-file#you-have-a-cloud-version--are-you-truly-open-source"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="underline decoration-1 underline-offset-2"
                  >
                    fully open source
                  </a>
                </div>
              </div>
            </div>

            {/* Self-hosted vs Cloud comparison */}
            <div className="w-full md:w-1/2 mt-8 md:mt-0 md:flex md:items-start">
              <div className="w-full min-w-0 rounded-xl border border-[#ffffff15] bg-[#ffffff05] p-3 sm:p-4">
                <table className="w-full table-fixed border-separate border-spacing-0 text-sm md:text-base">
                  <caption className="sr-only">
                    Self-hosted Databasus compared with Databasus Cloud
                  </caption>
                  <thead>
                    <tr>
                      <th
                        className="w-[34%] md:w-[22%] align-middle"
                        aria-hidden={true}
                      />
                      <th
                        scope="col"
                        className="px-2 py-2.5 md:px-3 md:py-3 text-center align-middle leading-none whitespace-nowrap font-medium text-gray-400"
                      >
                        Self-hosted
                      </th>
                      <th
                        scope="col"
                        className="px-2 py-2.5 md:px-3 md:py-3 text-center align-middle leading-none whitespace-nowrap font-semibold text-blue-400 border-l border-[#0d6efd]/30"
                      >
                        Cloud
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {[
                      {
                        feature: "Uptime",
                        self: "Up to you",
                        cloud: "24/7",
                      },
                      {
                        feature: "Price",
                        self: "VPS + maintenance + S3 + traffic",
                        cloud: "From $25/mo",
                      },
                      {
                        feature: "Updates",
                        self: "Up to you",
                        cloud: "Automatic",
                      },
                      {
                        feature: "Redundancy",
                        self: "Needs configuration",
                        cloud: "Built-in, 2×",
                      },
                      {
                        feature: "Verification",
                        self: "Needs configuration",
                        cloud: "On by default",
                      },
                      {
                        feature: "Support",
                        self: "Via GitHub issues",
                        cloud: "Via email",
                      },
                    ].map((row) => (
                      <tr key={row.feature} className="group">
                        <th
                          scope="row"
                          className="px-2 py-2.5 md:px-3 md:py-3 text-left align-middle font-medium text-gray-300 border-t border-[#ffffff0d] break-words"
                        >
                          {row.feature}
                        </th>
                        <td
                          className={`px-2 py-2.5 md:px-3 md:py-3 text-center align-middle border-t border-[#ffffff0d] ${
                            row.feature === "Price"
                              ? "text-red-500 font-medium"
                              : "text-gray-500"
                          }`}
                        >
                          {row.self}
                        </td>
                        <td className="px-2 py-2.5 md:px-3 md:py-3 text-center align-middle text-white font-medium border-t border-[#ffffff0d] border-l border-[#0d6efd]/30">
                          <span className="inline-flex items-center gap-1.5">
                            {row.feature === "Uptime" ? (
                              <span className="flex h-4 w-4 shrink-0 items-center justify-center">
                                <span className="relative flex h-2.5 w-2.5">
                                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75" />
                                  <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-green-500" />
                                </span>
                              </span>
                            ) : (
                              <svg
                                aria-hidden={true}
                                width="16"
                                height="16"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                strokeWidth="2.5"
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                className="shrink-0 text-[#0d6efd]"
                              >
                                <path d="M20 6 9 17l-5-5" />
                              </svg>
                            )}
                            <span>{row.cloud}</span>
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          </div>

          {/* Dashboard Screenshot */}
          <div className="relative mx-auto max-w-[1200px]">
            <div>
              <img
                src="/images/index/dashboard.svg"
                alt="Databasus dashboard interface"
                width={980}
                height={620}
                className="w-full h-auto"
                loading="eager"
                fetchPriority="high"
              />
            </div>
          </div>
        </div>
      </main>

      {/* PRICING SECTION */}
      <section id="pricing" className="pb-12 md:pb-20 px-4 md:px-6 lg:px-0">
        <div className="mx-auto w-full max-w-[1000px] 2xl:max-w-[1200px]">
          <div className="text-center mb-10 md:mb-14">
            <div className="inline-flex items-center justify-center px-3 md:px-4 py-1 md:py-1.5 rounded-lg border border-[#ffffff20] mb-4 md:mb-6">
              <span className="text-sm font-medium">Pricing</span>
            </div>

            <h2 className="text-3xl md:text-4xl lg:text-5xl font-bold mb-4 md:mb-6 max-w-[600px] mx-auto">
              Simple pricing, cheaper&nbsp;than self-hosting
            </h2>

            <p className="text-sm sm:text-lg text-gray-200 max-w-[650px] mx-auto">
              Every plan is priced per database and includes a verified restore
              test on daily basis. Scale storage, backup frequency and retention
              as you grow — usually cheaper than running it yourself.
            </p>
          </div>

          <div className="mx-auto w-full">
            {/* Mobile: stacked plan cards */}
            <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 md:hidden">
              {plans.map((plan, planIndex) => (
                <div
                  key={plan.name}
                  className={`flex flex-col rounded-2xl border p-5 ${
                    plan.featured
                      ? "border-[#0d6efd] bg-[#0d6efd]/[0.06] shadow-[0_0_40px_-12px_rgba(13,110,253,0.6)]"
                      : "border-[#ffffff15] bg-[#ffffff05]"
                  }`}
                >
                  <h3 className="text-lg font-semibold text-white">
                    {plan.name}
                  </h3>

                  <div className="mt-3 flex items-baseline gap-1">
                    {plan.from && (
                      <span className="text-sm text-gray-400">from</span>
                    )}
                    <span className="text-3xl font-bold text-white">
                      {plan.price}
                    </span>
                    {plan.period && (
                      <span className="text-gray-400">{plan.period}</span>
                    )}
                  </div>

                  <a
                    href={plan.cta.href}
                    target={
                      plan.cta.href.startsWith("http") ? "_blank" : undefined
                    }
                    rel={
                      plan.cta.href.startsWith("http")
                        ? "noopener noreferrer"
                        : undefined
                    }
                    className={`mt-5 inline-flex w-full items-center justify-center rounded-lg px-4 py-2.5 text-sm font-medium transition-colors ${
                      plan.featured
                        ? "bg-[#0d6efd] text-white hover:opacity-80"
                        : "border border-[#ffffff20] text-white hover:bg-[#ffffff10]"
                    }`}
                  >
                    {plan.cta.label}
                  </a>

                  <ul className="mt-5 space-y-3 text-sm">
                    {rows.map((row) => {
                      const value = row.values[planIndex];
                      const isString = typeof value === "string";
                      const ok = isString ? true : value;

                      return (
                        <li
                          key={row.label}
                          className="flex items-start gap-2.5"
                        >
                          {ok ? (
                            <svg
                              aria-hidden={true}
                              viewBox="0 0 24 24"
                              fill="none"
                              stroke="currentColor"
                              strokeWidth="2.5"
                              strokeLinecap="round"
                              strokeLinejoin="round"
                              className="mt-0.5 h-[18px] w-[18px] shrink-0 text-[#0d6efd]"
                            >
                              <path d="M20 6 9 17l-5-5" />
                            </svg>
                          ) : (
                            <svg
                              aria-hidden={true}
                              viewBox="0 0 24 24"
                              fill="none"
                              stroke="currentColor"
                              strokeWidth="2.5"
                              strokeLinecap="round"
                              strokeLinejoin="round"
                              className="mt-0.5 h-[18px] w-[18px] shrink-0 text-red-500"
                            >
                              <path d="M18 6 6 18M6 6l12 12" />
                            </svg>
                          )}

                          <span
                            className={ok ? "text-gray-200" : "text-gray-500"}
                          >
                            {isString ? (
                              <>
                                <span className="text-gray-400">
                                  {row.label}:{" "}
                                </span>
                                <span className="font-medium text-white">
                                  {value}
                                </span>
                              </>
                            ) : (
                              row.label
                            )}
                            {row.tooltip && (
                              <span
                                tabIndex={0}
                                aria-label={row.tooltip}
                                className="group/tip relative ml-1 inline cursor-help align-middle"
                              >
                                <svg
                                  aria-hidden={true}
                                  width="15"
                                  height="15"
                                  viewBox="0 0 24 24"
                                  fill="none"
                                  stroke="currentColor"
                                  strokeWidth="2"
                                  strokeLinecap="round"
                                  strokeLinejoin="round"
                                  className="inline align-[-0.18em] text-gray-500 transition-colors group-hover/tip:text-gray-300"
                                >
                                  <circle cx="12" cy="12" r="10" />
                                  <path d="M12 16v-4M12 8h.01" />
                                </svg>
                                <span className="pointer-events-none absolute bottom-full left-0 z-20 mb-2 w-56 max-w-[70vw] rounded-lg border border-[#ffffff20] bg-[#0C0E13] px-3 py-2 text-left text-xs font-normal leading-relaxed text-gray-300 opacity-0 shadow-xl transition-opacity duration-150 group-hover/tip:opacity-100 group-focus/tip:opacity-100">
                                  {row.tooltip}
                                </span>
                              </span>
                            )}
                          </span>
                        </li>
                      );
                    })}
                  </ul>
                </div>
              ))}
            </div>

            {/* Desktop: comparison table */}
            <div className="hidden rounded-2xl border border-[#ffffff15] bg-[#ffffff05] p-3 sm:p-5 md:block md:p-6">
              <table className="w-full table-fixed border-separate border-spacing-0">
                <caption className="sr-only">
                  Databasus Cloud pricing plans compared by feature
                </caption>

                <colgroup>
                  <col className="w-[35%] md:w-[31%]" />
                  <col className="w-[16.25%] md:w-[17.25%]" />
                  <col className="w-[16.25%] md:w-[17.25%]" />
                  <col className="w-[16.25%] md:w-[17.25%]" />
                  <col className="w-[16.25%] md:w-[17.25%]" />
                </colgroup>

                <thead>
                  <tr>
                    <th aria-hidden={true} />
                    {plans.map((plan) => (
                      <th
                        key={plan.name}
                        scope="col"
                        className={`px-1 pt-2 pb-3 text-center align-bottom md:px-2 ${
                          plan.featured
                            ? "rounded-t-xl border-x border-t border-[#0d6efd]/40 bg-[#0d6efd]/[0.06]"
                            : ""
                        }`}
                      >
                        <div className="text-[13px] font-semibold text-white sm:text-sm md:text-base">
                          {plan.name}
                        </div>

                        <div className="mt-1 flex items-baseline justify-center gap-0.5">
                          {plan.from && (
                            <span className="text-[11px] text-gray-400 md:text-sm">
                              from
                            </span>
                          )}
                          <span className="text-base font-bold text-white sm:text-lg md:text-2xl">
                            {plan.price}
                          </span>
                          {plan.period && (
                            <span className="text-[11px] text-gray-400 md:text-sm">
                              {plan.period}
                            </span>
                          )}
                        </div>

                        <a
                          href={plan.cta.href}
                          target={
                            plan.cta.href.startsWith("http")
                              ? "_blank"
                              : undefined
                          }
                          rel={
                            plan.cta.href.startsWith("http")
                              ? "noopener noreferrer"
                              : undefined
                          }
                          aria-label={`${plan.cta.label} — ${plan.name}`}
                          className={`mt-2.5 inline-flex w-full items-center justify-center gap-1 rounded-lg px-1 py-1.5 text-xs font-medium transition-colors md:mt-3 md:px-2 md:py-2 md:text-sm ${
                            plan.featured
                              ? "bg-[#0d6efd] text-white hover:opacity-80"
                              : "border border-[#ffffff20] text-white hover:bg-[#ffffff10]"
                          }`}
                        >
                          <span className="hidden sm:inline">
                            {plan.cta.label}
                          </span>
                          <svg
                            aria-hidden={true}
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            strokeWidth="2"
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            className="h-4 w-4 sm:hidden"
                          >
                            <path d="M5 12h14M12 5l7 7-7 7" />
                          </svg>
                        </a>
                      </th>
                    ))}
                  </tr>
                </thead>

                <tbody>
                  {rows.map((row, rowIndex) => {
                    const isLast = rowIndex === rows.length - 1;

                    return (
                      <tr key={row.label}>
                        <th
                          scope="row"
                          className="break-words border-t border-[#ffffff0d] py-2.5 pr-2 text-left align-middle text-xs font-medium text-gray-300 md:py-3 md:text-sm"
                        >
                          {row.label}
                          {row.tooltip && (
                            <span
                              tabIndex={0}
                              aria-label={row.tooltip}
                              className="group/tip relative ml-1 inline cursor-help align-middle"
                            >
                              <svg
                                aria-hidden={true}
                                width="15"
                                height="15"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                strokeWidth="2"
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                className="inline align-[-0.18em] text-gray-500 transition-colors group-hover/tip:text-gray-300"
                              >
                                <circle cx="12" cy="12" r="10" />
                                <path d="M12 16v-4M12 8h.01" />
                              </svg>
                              <span className="pointer-events-none absolute bottom-full left-0 z-20 mb-2 w-56 max-w-[70vw] rounded-lg border border-[#ffffff20] bg-[#0C0E13] px-3 py-2 text-left text-xs font-normal leading-relaxed text-gray-300 opacity-0 shadow-xl transition-opacity duration-150 group-hover/tip:opacity-100 group-focus/tip:opacity-100">
                                {row.tooltip}
                              </span>
                            </span>
                          )}
                        </th>

                        {row.values.map((value, planIndex) => {
                          const featured = plans[planIndex].featured;

                          return (
                            <td
                              key={planIndex}
                              className={`border-t py-2.5 text-center align-middle md:py-3 ${
                                featured
                                  ? `border-x border-[#0d6efd]/40 bg-[#0d6efd]/[0.06] ${
                                      isLast ? "rounded-b-xl border-b" : ""
                                    }`
                                  : "border-[#ffffff0d]"
                              }`}
                            >
                              {typeof value === "string" ? (
                                <span className="text-xs font-medium text-white md:text-sm">
                                  {value}
                                </span>
                              ) : value ? (
                                <svg
                                  aria-hidden={true}
                                  viewBox="0 0 24 24"
                                  fill="none"
                                  stroke="currentColor"
                                  strokeWidth="2.5"
                                  strokeLinecap="round"
                                  strokeLinejoin="round"
                                  className="inline h-4 w-4 text-[#0d6efd] md:h-[18px] md:w-[18px]"
                                >
                                  <path d="M20 6 9 17l-5-5" />
                                </svg>
                              ) : (
                                <svg
                                  aria-hidden={true}
                                  viewBox="0 0 24 24"
                                  fill="none"
                                  stroke="currentColor"
                                  strokeWidth="2.5"
                                  strokeLinecap="round"
                                  strokeLinejoin="round"
                                  className="inline h-4 w-4 text-red-500 md:h-[18px] md:w-[18px]"
                                >
                                  <path d="M18 6 6 18M6 6l12 12" />
                                </svg>
                              )}
                            </td>
                          );
                        })}
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </section>

      {/* FAQ SECTION */}
      <section id="faq" className="pb-12 md:pb-20 px-4 md:px-6 lg:px-0">
        <div className="mx-auto w-full max-w-[1000px] 2xl:max-w-[1200px]">
          <div className="text-center mb-8 md:mb-12">
            <div className="inline-flex items-center justify-center px-3 md:px-4 py-1 md:py-1.5 rounded-lg border border-[#ffffff20] mb-4 md:mb-6">
              <span className="text-sm font-medium">FAQ</span>
            </div>

            <h2 className="text-3xl md:text-4xl lg:text-5xl font-bold mb-4 md:mb-6">
              Frequent questions
            </h2>

            <p className="text-base md:text-lg text-gray-200 max-w-[600px] mx-auto">
              Common questions about Databasus Cloud, pricing and how it
              compares to self-hosted
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 md:gap-8">
            <CloudFaqItem
              number="1"
              question="Can I change my plan later?"
              answer={
                "Yes. You can upgrade or downgrade at any time. Extra storage becomes available right after you upgrade. You can move to a smaller plan whenever you use less than expected. There are no penalties or lock-in periods."
              }
            />
            <CloudFaqItem
              number="2"
              question="Which backup types do you support?"
              answer={
                "We support both logical and physical backups. Logical backups work well for small databases. For databases over 50 GB we use physical backups. They are much faster to back up and restore. They also support incremental backups."
              }
            />
            <CloudFaqItem
              number="3"
              question="Are there any differences between Databasus Cloud and self-hosted?"
              answer={
                "No. Databasus Cloud has the exact same features as the self-hosted version. There are no paywalled extras, no premium tiers and no hidden limitations. Databasus is fully open source under the Apache 2.0 license. It is not an 'open core' model.\n\nWith cloud we handle the infrastructure, uptime and updates. You focus on your work instead of maintaining servers. You can switch between cloud and self-hosted at any time. There is no vendor lock-in."
              }
            />
          </div>
        </div>
      </section>

      {/* FOOTER */}
      <footer className="py-8 md:py-12 border-t border-[#ffffff20] px-4 md:px-6 lg:px-0">
        <div className="mx-auto w-full max-w-[1000px] 2xl:max-w-[1200px]">
          <div className="flex flex-col items-center">
            <a href="/" className="flex items-center gap-2.5 mb-6">
              <img
                src="/logo.svg"
                alt="Databasus logo"
                width={32}
                height={32}
                className="h-7 w-7 md:h-8 md:w-8"
              />

              <span className="text-base md:text-lg font-semibold">
                Databasus
              </span>
            </a>

            <div className="flex flex-col gap-3 mb-4 text-sm md:text-base">
              {/* First row - General links */}
              <div className="flex flex-wrap items-center justify-center gap-4 md:gap-6">
                <a
                  href="/installation"
                  className="hover:text-gray-200 transition-colors"
                >
                  Documentation
                </a>
                <a
                  href="/privacy"
                  className="hover:text-gray-200 transition-colors"
                >
                  Privacy
                </a>
                <a
                  href="https://github.com/databasus/databasus"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="hover:text-gray-200 transition-colors"
                >
                  GitHub
                </a>
                <a
                  href="https://t.me/databasus_community"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="hover:text-gray-200 transition-colors"
                >
                  Community
                </a>
                <a
                  href="https://rostislav-dugin.com"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="hover:text-gray-200 transition-colors"
                >
                  Developer
                </a>
              </div>

              {/* Second row - Cloud legal links */}
              <div className="flex flex-wrap items-center justify-center gap-4 md:gap-6">
                <a
                  href="/privacy-cloud"
                  className="hover:text-gray-200 transition-colors"
                >
                  Privacy policy (cloud)
                </a>
                <a
                  href="/terms-of-use-cloud"
                  className="hover:text-gray-200 transition-colors"
                >
                  Terms of use (cloud)
                </a>
              </div>
            </div>

            <a
              href="mailto:info@databasus.com"
              className="hover:text-gray-200 transition-colors text-sm md:text-base mb-4"
            >
              info@databasus.com
            </a>

            <p className="text-gray-400 text-sm md:text-base text-center">
              © 2026 Databasus™. All rights reserved.
            </p>
          </div>
        </div>
      </footer>
    </div>
  );
}

function CloudFaqItem({
  number,
  question,
  answer,
}: {
  number: string;
  question: string;
  answer: string;
}) {
  const paragraphs = answer.split("\n\n");

  return (
    <div className="rounded-lg border border-[#ffffff20] p-4 md:p-6">
      <div className="flex items-center justify-center w-6 h-6 rounded border border-[#ffffff20] text-sm font-semibold mb-3 md:mb-4">
        {number}
      </div>

      <h3 className="text-base md:text-lg font-bold mb-2 md:mb-3">
        {question}
      </h3>

      <div className="text-gray-400 text-sm md:text-base space-y-3">
        {paragraphs.map((p, i) => (
          <p key={i}>{p}</p>
        ))}
      </div>
    </div>
  );
}
