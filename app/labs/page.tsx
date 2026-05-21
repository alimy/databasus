import type { Metadata } from "next";
import LabsBriefForm from "../components/LabsBriefForm";

export const metadata: Metadata = {
  title: "Databasus Labs | DBA-as-a-service",
  description:
    "External PostgreSQL DBA by Databasus Labs: we take responsibility for your backups, verify restores, run audits, consult and respond to emergencies within 15 minutes — even at 3 AM.",
  keywords: [
    "PostgreSQL DBA services",
    "external PostgreSQL DBA",
    "part-time DBA",
    "PostgreSQL backup",
    "PostgreSQL consultation",
    "PostgreSQL audit",
    "restore verification",
    "backup restore audit",
    "disaster recovery",
    "emergency PostgreSQL recovery",
    "PostgreSQL incident response",
    "remote DBA",
    "managed PostgreSQL",
    "self-hosted Databasus",
    "pgBackRest",
    "Point-In-Time Recovery",
    "Databasus Labs",
    "Databasus",
  ],
  robots: "index, follow",
  alternates: {
    canonical: "https://databasus.com/labs",
  },
  openGraph: {
    type: "website",
    url: "https://databasus.com/labs",
    title: "Databasus Labs | DBA-as-a-service",
    description:
      "External PostgreSQL DBA by Databasus Labs: we take responsibility for your backups, verify restores, run audits, consult and respond to emergencies within 15 minutes — even at 3 AM.",
    siteName: "Databasus",
    locale: "en_US",
  },
  twitter: {
    title: "Databasus Labs | DBA-as-a-service",
    description:
      "External PostgreSQL DBA by Databasus Labs: we take responsibility for your backups, verify restores, run audits, consult and respond to emergencies within 15 minutes — even at 3 AM.",
  },
};

function Check({
  className = "mt-0.5 h-4 w-4 shrink-0 text-emerald-400",
}: {
  className?: string;
}) {
  return (
    <svg
      className={className}
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="m5 10.5 3 3 7-7" />
    </svg>
  );
}

type Feature = string | { text: string; bold: true };

interface Service {
  title: string;
  subtitle: string;
  price: string;
  priceUnit: string;
  priceFootnote?: string;
  features: Feature[];
  featured?: boolean;
}

const SERVICES: Service[] = [
  {
    title: "Manage Databasus in your infrastructure",
    subtitle:
      "I install, configure and monitor Databasus in your company's infrastructure. If you want managed Databasus, but don't want cloud",
    price: "$250",
    priceUnit: "per month",
    features: [
      "Installation and configuration within your infrastructure",
      "Continuous monitoring 24x7 (backups are running, enough storage)",
      "Restore verification for each database",
      "Support included (without SLA)",
    ],
  },
  {
    title: "Consultation",
    subtitle:
      "Calls or messaging for any of your questions. Related to databases, processes, best practices or anything else my experience can help you with",
    price: "$100",
    priceUnit: "per hour",
    features: [
      "PostgreSQL questions and architecture advice",
      "Investigation of a specific problem you’re facing",
      "Calls or async messaging — whatever works for you",
      "Billed hourly, only for the time used",
    ],
  },
  {
    title: "Full care of your backups",
    subtitle:
      "I take full responsibility for your company's backups. So at any moment I am ready to restore your data and guarantee it is safe 24x7",
    price: "$500",
    priceUnit: "per month",
    priceFootnote: "Per database",
    featured: true,
    features: [
      "Configure backups for your infrastructure and data layout (both self-hosted and cloud DBs)",
      "Set up backups + restore verification for every DB with Point-In-Time-Recovery readiness (Databasus or pgBackRest)",
      {
        text: "Guaranteed emergency response within 15 minutes, even at 3 AM",
        bold: true,
      },
      "Monthly reports on backup health",
    ],
  },
  {
    title: "Backup & restore audit",
    subtitle:
      "Independent one-off review of your company's process. If you have a team and just want to double-check everything is set up correctly",
    price: "$990",
    priceUnit: "one-time",
    features: [
      "Review current backup processes and tooling",
      "Assess restore readiness (can you actually recover?)",
      "Recommend and set up the right backup tools",
      "Written report with findings and an action plan",
    ],
  },
  {
    title: "DBA services",
    subtitle:
      "Your part-time DBA for database management. Optimizations, version upgrades, migrations and any other database-related tasks",
    price: "$100",
    priceUnit: "per hour",
    features: [
      "Performance monitoring and query tuning",
      "Version upgrades and migrations",
      "Schema, indexing and configuration review",
      "Ad-hoc DBA tasks as they come up",
    ],
  },
  {
    title: "Emergency help",
    subtitle:
      "You faced an incident and need immediate help? I jump into your problem and try to fix it by hand instead of just advising",
    price: "$150",
    priceUnit: "per hour",
    priceFootnote: 'Included with "Full care"',
    features: [
      "Rapid response for data loss / corruption / failed restore",
      "Hands-on recovery, not just advice",
      "Specifically covers nights and off-hours (*you need to get my phone number via message in advance to call at night)",
      "Post-incident write-up so it doesn’t happen again",
    ],
  },
];

function ServiceCard({ service }: { service: Service }) {
  const f = service.featured;
  return (
    <div
      className={`group relative flex h-full flex-col rounded-2xl border p-7 transition-[transform,border-color] duration-200 hover:-translate-y-1 ${
        f
          ? "border-[#155DFC] bg-[#155DFC]/[0.06]"
          : "border-[#ffffff20] bg-[#0C0E13] hover:border-[#ffffff40]"
      }`}
    >
      <h3 className="min-h-[3.5rem] text-xl leading-snug font-bold text-white">
        {service.title}
      </h3>
      <p className={`mt-1 text-base ${f ? "text-[#7da8f5]" : "text-gray-500"}`}>
        {service.subtitle}
      </p>

      <ul
        className={`mt-6 flex-1 list-none space-y-3 border-t pt-6 pl-0 text-base leading-snug ${
          f
            ? "border-[#155DFC]/30 text-gray-200"
            : "border-[#ffffff15] text-gray-300"
        }`}
      >
        {service.features.map((feat, i) => {
          const text = typeof feat === "string" ? feat : feat.text;
          const bold = typeof feat !== "string" && feat.bold;
          return (
            <li key={i} className="flex gap-2.5">
              <span
                aria-hidden="true"
                className={`mt-2 inline-block h-1.5 w-1.5 shrink-0 rounded-full ${
                  f ? "bg-[#155DFC]" : "bg-gray-500"
                }`}
              />
              <span className={bold ? "font-semibold text-white" : undefined}>
                {text}
              </span>
            </li>
          );
        })}
      </ul>

      <div
        className={`mt-6 border-t pt-6 ${
          f ? "border-[#155DFC]/30" : "border-[#ffffff15]"
        }`}
      >
        <div className="flex items-baseline gap-2">
          <span className="text-4xl font-extrabold text-white tabular-nums leading-none">
            {service.price}
          </span>
          <span
            className={`text-base ${f ? "text-gray-300" : "text-gray-400"}`}
          >
            {service.priceUnit}
          </span>
        </div>
        <p
          className={`mt-2 text-base ${f ? "text-[#7da8f5]" : "text-gray-500"}`}
        >
          {service.priceFootnote || " "}
        </p>
      </div>
    </div>
  );
}

export default function LabsPage() {
  return (
    <>
      {/* JSON-LD Structured Data */}
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{
          __html: JSON.stringify({
            "@context": "https://schema.org",
            "@type": "ProfessionalService",
            name: "Databasus Labs",
            description:
              "External PostgreSQL DBA by Rostislav Dugin: backup setup, restore verification, audits, consultation, part-time DBA and emergency response within 15 minutes.",
            url: "https://databasus.com/labs",
            image: "https://databasus.com/images/index/rostislav.png",
            logo: "https://databasus.com/logo.svg",
            areaServed: "Worldwide",
            provider: {
              "@type": "Organization",
              name: "Databasus",
              logo: {
                "@type": "ImageObject",
                url: "https://databasus.com/logo.svg",
              },
            },
            hasOfferCatalog: {
              "@type": "OfferCatalog",
              name: "DBA Services",
              itemListElement: SERVICES.map((s) => ({
                "@type": "Offer",
                itemOffered: { "@type": "Service", name: s.title },
              })),
            },
          }),
        }}
      />

      <div className="overflow-x-hidden">
        {/* Navbar (floating, centered, blurred pill) */}
        <header className="fixed top-0 right-0 left-0 z-50 flex justify-center px-4 pt-5 sm:px-6 lg:px-0">
          <div className="mx-auto w-full max-w-[1000px]">
            <nav className="relative flex items-center justify-between rounded-xl border border-[#ffffff20] bg-[#0C0E13]/20 px-3 py-2 backdrop-blur-md">
              <a href="/" className="flex min-h-[42px] items-center gap-2.5">
                <img
                  src="/logo.svg"
                  alt="Databasus Labs logo"
                  width={32}
                  height={32}
                  className="h-8 w-8"
                  fetchPriority="high"
                  loading="eager"
                />
                <span className="pl-1 text-lg font-semibold">
                  Databasus Labs
                </span>
              </a>
              <a
                href="#contact"
                className="cursor-pointer rounded-lg bg-white px-4 py-2 text-sm font-semibold text-[#0F1115] transition-colors hover:bg-gray-200 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#0d6efd]"
              >
                Message me
              </a>
            </nav>
          </div>
        </header>

        {/* Hero */}
        <main className="relative overflow-hidden pt-[68px]">
          <div className="relative mx-auto w-full max-w-[1000px] px-4 pt-[100px] pb-[100px] sm:px-6 lg:px-0">
            {/* Background ellipse */}
            <div className="relative">
              <div className="absolute top-0 left-1/2 -z-10 h-[900px] w-[900px] -translate-x-1/2 -translate-y-1/4 rounded-full bg-[#155dfc]/4 blur-3xl"></div>
            </div>

            <div className="grid grid-cols-1 items-start gap-16 lg:grid-cols-2">
              {/* Left: pitch */}
              <div>
                <h1 className="text-2xl leading-snug font-extrabold text-white sm:text-3xl lg:text-[26px] lg:leading-9">
                  Need someone to worry about your backups? I will{" "}
                  <span className="text-white underline decoration-[#0d6efd] decoration-2 underline-offset-4">
                    guarantee restore
                  </span>{" "}
                  with a 15-minute response time
                </h1>

                <p className="mt-6 max-w-md text-lg leading-relaxed text-gray-300">
                  Alongside developing Databasus, I provide external DBA
                  services: I{" "}
                  <span className="underline underline-offset-2">
                    take responsibility for backups in your company
                  </span>
                  , verify your team&apos;s restore procedures, run an audit and
                  make sure your data is safe in case of an emergency
                </p>

                <ul className="mt-8 space-y-3 text-lg">
                  <li className="flex items-center gap-3 text-gray-200">
                    <Check className="h-5 w-5 shrink-0 text-emerald-400" />I
                    care about reliable restores, not just backups
                  </li>
                  <li className="flex items-center gap-3 text-gray-200">
                    <Check className="h-5 w-5 shrink-0 text-emerald-400" />
                    25+ incidents studied over 5 years of running backups
                  </li>
                  <li className="flex items-center gap-3 text-gray-200">
                    <Check className="h-5 w-5 shrink-0 text-emerald-400" />
                    DBA and DevOps team included on demand
                  </li>
                  <li className="flex items-center gap-3 text-gray-200">
                    <Check className="h-5 w-5 shrink-0 text-emerald-400" />
                    Work inside your infra under an NDA
                  </li>
                </ul>

                <div className="mt-10 flex flex-col gap-3">
                  <a
                    href="#services"
                    className="w-full cursor-pointer rounded-lg bg-white px-5 py-3 text-center font-semibold text-[#0F1115] transition-colors hover:bg-gray-200 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#0d6efd] sm:max-w-[300px]"
                  >
                    How can I help you?
                  </a>
                  <a
                    href="#contact"
                    className="w-full cursor-pointer rounded-lg border border-[#ffffff20] bg-[#0C0E13] px-5 py-3 text-center font-semibold text-white transition-colors hover:bg-[#15171d] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#0d6efd] sm:max-w-[300px]"
                  >
                    Message me
                  </a>
                </div>
              </div>

              {/* Right: credibility card */}
              <aside className="w-full max-w-md justify-self-center overflow-hidden rounded-2xl border border-[#ffffff20] bg-[#0C0E13] lg:justify-self-end">
                {/* Photo */}
                <div className="relative flex justify-center px-8 py-10">
                  <div
                    className="pointer-events-none absolute inset-0 flex items-center justify-center"
                    aria-hidden="true"
                  >
                    <div className="h-44 w-44 rounded-full bg-[#0d6efd]/15 blur-3xl"></div>
                  </div>
                  <img
                    src="/images/index/rostislav.png"
                    width={160}
                    height={160}
                    loading="eager"
                    className="relative h-40 w-40 rounded-full object-cover ring-1 ring-[#ffffff20]"
                    alt="Rostislav Dugin"
                  />
                </div>

                {/* Name + role */}
                <div className="flex items-center gap-2 border-t border-[#ffffff20] px-6 py-4 text-lg">
                  <span className="font-bold text-white">Rostislav Dugin</span>
                  <span className="text-gray-600">·</span>
                  <img
                    src="/logo.svg"
                    alt=""
                    aria-hidden="true"
                    className="h-5 w-5"
                  />
                  <span className="text-gray-400">Developer of Databasus</span>
                </div>

                {/* Stats */}
                <div className="grid grid-cols-2 divide-x divide-[#ffffff20] border-t border-[#ffffff20]">
                  <div className="px-6 py-5">
                    <div className="flex items-center gap-2 text-2xl font-extrabold text-white tabular-nums">
                      {">"}30,000
                      <span className="relative ml-3 inline-flex h-1.5 w-1.5">
                        <span className="absolute top-1/2 left-1/2 h-4 w-4 -translate-x-1/2 -translate-y-1/2 animate-ping rounded-full bg-emerald-400 opacity-75"></span>
                        <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-emerald-400"></span>
                      </span>
                    </div>
                    <div className="mt-1 text-base text-gray-400">
                      PostgreSQL databases managed via self-hosted Databasus
                      right now
                    </div>
                  </div>
                  <div className="px-6 py-5">
                    <div className="text-2xl font-extrabold text-white tabular-nums">
                      5 years
                    </div>
                    <div className="mt-1 text-base text-gray-400">
                      backing up production PostgreSQL and dealing with
                      incidents
                    </div>
                  </div>
                </div>

                {/* View CV */}
                <a
                  href="https://rostislav-dugin.com"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center justify-between border-t border-[#ffffff20] px-6 py-4 text-gray-200 transition-colors hover:bg-white/5 focus-visible:outline focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-[#0d6efd]"
                >
                  <span className="flex items-center gap-2">
                    <svg
                      className="h-4 w-4 text-gray-400"
                      viewBox="0 0 20 20"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth={1.6}
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      aria-hidden="true"
                    >
                      <path d="M11.5 2.5H6a1.5 1.5 0 0 0-1.5 1.5v12A1.5 1.5 0 0 0 6 17.5h8a1.5 1.5 0 0 0 1.5-1.5V6.5L11.5 2.5Z" />
                      <path d="M11.5 2.5v4h4" />
                    </svg>
                    <span className="font-medium">View CV</span>
                  </span>
                  <svg
                    className="h-4 w-4 text-gray-400"
                    viewBox="0 0 20 20"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth={1.8}
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    aria-hidden="true"
                  >
                    <path d="M7 13 13 7" />
                    <path d="M7.5 7H13v5.5" />
                  </svg>
                </a>
              </aside>
            </div>
          </div>
        </main>

        {/* Services */}
        <section
          id="services"
          className="relative mx-auto w-full max-w-[1000px] scroll-mt-24 px-4 pb-[120px] sm:px-6 lg:px-0"
        >
          <div className="max-w-2xl">
            <h2 className="text-2xl leading-snug font-extrabold text-white sm:text-3xl lg:text-[26px] lg:leading-9">
              What can I do for you?
            </h2>
            <p className="mt-3 text-lg text-gray-300">
              I work as an external DBA / backup-focused CTO for your team.{" "}
              <span className="text-white">
                If you&apos;re not satisfied — I return 100% of the payment, no
                questions asked. By the way,{" "}
                <span className="underline underline-offset-2 decoration-[#0d6efd]">
                  money from Databasus Labs goes to Databasus open source
                  project development.
                </span>
              </span>
            </p>
          </div>

          <div className="mt-12 grid grid-cols-1 items-stretch gap-5 sm:grid-cols-2 lg:grid-cols-3">
            {SERVICES.map((service) => (
              <ServiceCard key={service.title} service={service} />
            ))}
          </div>

          <div className="mt-12 flex justify-center">
            <a
              href="https://t.me/rostislav_dugin"
              target="_blank"
              rel="noopener noreferrer"
              className="w-full cursor-pointer rounded-lg bg-white px-5 py-3 text-center font-semibold text-[#0F1115] transition-colors hover:bg-gray-200 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#0d6efd] sm:max-w-[300px]"
            >
              Contact me
            </a>
          </div>
        </section>

        {/* When teams ask for help */}
        <section
          id="when"
          className="relative mx-auto w-full max-w-[1000px] scroll-mt-24 px-4 pb-[120px] sm:px-6 lg:px-0"
        >
          <div className="max-w-2xl">
            <h2 className="text-2xl leading-snug font-extrabold text-white sm:text-3xl lg:text-[26px] lg:leading-9">
              How to tell if you need my help?
            </h2>
            <p className="mt-3 text-lg text-gray-300">
              A few situations I see again and again. If one of them sounds like
              you —{" "}
              <span className="text-white">
                message me and I&apos;ll tell you which option fits.
              </span>
            </p>
          </div>

          <div className="mt-12 grid grid-cols-1 gap-5 md:grid-cols-2">
            {/* 01 */}
            <article className="group rounded-2xl border border-[#ffffff20] bg-[#0C0E13] p-7 transition-[transform,border-color] duration-200 hover:-translate-y-1 hover:border-[#ffffff40]">
              <div className="flex items-center gap-3">
                <span className="text-2xl font-extrabold text-[#155DFC] tabular-nums">
                  01
                </span>
                <span className="text-base font-semibold tracking-wider text-gray-500 uppercase">
                  No one owns backups
                </span>
              </div>
              <h3 className="mt-4 text-xl leading-snug font-bold text-white">
                &quot;Nobody is responsible for our backups.&quot;
              </h3>
              <p className="mt-2 text-base leading-relaxed text-gray-400">
                PostgreSQL runs in production, but no single person can
                guarantee you won&apos;t lose data. I take ownership: what is
                backed up, who is responsible and exactly what happens at 3 AM
                when something breaks.
              </p>
            </article>

            {/* 02 */}
            <article className="group rounded-2xl border border-[#ffffff20] bg-[#0C0E13] p-7 transition-[transform,border-color] duration-200 hover:-translate-y-1 hover:border-[#ffffff40]">
              <div className="flex items-center gap-3">
                <span className="text-2xl font-extrabold text-[#155DFC] tabular-nums">
                  02
                </span>
                <span className="text-base font-semibold tracking-wider text-gray-500 uppercase">
                  CEO / non-DBA CTO
                </span>
              </div>
              <h3 className="mt-4 text-xl leading-snug font-bold text-white">
                &quot;I&apos;m a CEO and I don&apos;t know what to do.&quot;
              </h3>
              <p className="mt-2 text-base leading-relaxed text-gray-400">
                You have PostgreSQL in production, but no DBA. Backups exist
                somewhere, monitoring is unclear and nobody knows how long
                recovery would actually take.
              </p>
            </article>

            {/* 03 */}
            <article className="group rounded-2xl border border-[#ffffff20] bg-[#0C0E13] p-7 transition-[transform,border-color] duration-200 hover:-translate-y-1 hover:border-[#ffffff40]">
              <div className="flex items-center gap-3">
                <span className="text-2xl font-extrabold text-[#155DFC] tabular-nums">
                  03
                </span>
                <span className="text-base font-semibold tracking-wider text-gray-500 uppercase">
                  Tech-literate team
                </span>
              </div>
              <h3 className="mt-4 text-xl leading-snug font-bold text-white">
                &quot;We have backups, but nobody tested the restore.&quot;
              </h3>
              <p className="mt-2 text-base leading-relaxed text-gray-400">
                A successful backup is not proof of recovery. I check whether
                your database can actually be restored — and how long it really
                takes under pressure.
              </p>
            </article>

            {/* 04 */}
            <article className="group rounded-2xl border border-[#ffffff20] bg-[#0C0E13] p-7 transition-[transform,border-color] duration-200 hover:-translate-y-1 hover:border-[#ffffff40]">
              <div className="flex items-center gap-3">
                <span className="text-2xl font-extrabold text-[#155DFC] tabular-nums">
                  04
                </span>
                <span className="text-base font-semibold tracking-wider text-gray-500 uppercase">
                  Databasus users
                </span>
              </div>
              <h3 className="mt-4 text-xl leading-snug font-bold text-white">
                &quot;I want Databasus, but I don&apos;t want to care.&quot;
              </h3>
              <p className="mt-2 text-base leading-relaxed text-gray-400">
                You want Databasus running, but you don&apos;t want to deal with
                it yourself. The cloud is not an option — it has to live inside
                your own infrastructure. I set it up there and take care of it
                for you.
              </p>
            </article>
          </div>
        </section>

        {/* Message me (contact + brief) */}
        <section
          id="contact"
          className="relative mx-auto w-full max-w-[1000px] scroll-mt-24 px-4 pb-[120px] sm:px-6 lg:px-0"
        >
          <div className="max-w-2xl">
            <h2 className="text-2xl leading-snug font-extrabold text-white sm:text-3xl lg:text-[26px] lg:leading-9">
              Message me
            </h2>
            <p className="mt-3 text-lg text-gray-300">
              Tell me your situation in a few words. Fill in the short brief
              below,{" "}
              <span className="text-white">
                copy it and send via Telegram or email
              </span>{" "}
              — I&apos;ll reply with which option fits.
            </p>
          </div>

          <div className="mt-12 grid grid-cols-1 gap-5 lg:grid-cols-3">
            {/* Direct contacts */}
            <article className="flex flex-col rounded-2xl border border-[#ffffff20] bg-[#0C0E13] p-7 lg:order-2">
              <h3 className="text-xl leading-snug font-bold text-white">
                Reach me directly
              </h3>
              <p className="mt-1.5 text-base text-gray-500">
                Usual response time — within a few hours
              </p>
              <div className="mt-6 flex flex-1 flex-col gap-3">
                <a
                  href="https://t.me/rostislav_dugin"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex flex-col gap-0.5 rounded-lg bg-white px-5 py-3 transition-colors hover:bg-gray-200 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#0d6efd]"
                >
                  <span className="text-base font-semibold text-[#0F1115]">
                    Telegram
                  </span>
                  <span className="text-base break-all text-gray-600">
                    @rostislav_dugin
                  </span>
                </a>
                <a
                  href="mailto:info@databasus.com"
                  className="flex flex-col gap-0.5 rounded-lg border border-[#ffffff20] bg-transparent px-5 py-3 transition-colors hover:bg-white/5 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#0d6efd]"
                >
                  <span className="text-base font-semibold text-white">
                    Email
                  </span>
                  <span className="text-base break-all text-gray-400">
                    info@databasus.com
                  </span>
                </a>
              </div>
            </article>

            {/* Brief builder */}
            <article className="rounded-2xl border border-[#ffffff20] bg-[#0C0E13] p-7 lg:order-1 lg:col-span-2">
              <h3 className="text-xl leading-snug font-bold text-white">
                Suggested brief before reaching out
              </h3>
              <p className="mt-1.5 text-base text-gray-500">
                Will help me get involved faster with fewer questions
              </p>
              <LabsBriefForm />
            </article>
          </div>
        </section>
      </div>
    </>
  );
}
