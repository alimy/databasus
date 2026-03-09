"use client";

import { useState } from "react";
import { createPortal } from "react-dom";

export function CloudDashboardButton({
  variant,
}: {
  variant: "navbar" | "hero";
}) {
  const [showDialog, setShowDialog] = useState(false);

  return (
    <>
      {variant === "navbar" ? (
        <button
          onClick={() => setShowDialog(true)}
          className="flex items-center gap-2 hover:opacity-70 rounded-lg px-2 md:px-3 py-2 text-[14px] border border-[#0d6efd] bg-[#0d6efd] transition-colors"
        >
          Dashboard
        </button>
      ) : (
        <button
          onClick={() => setShowDialog(true)}
          className="w-full sm:w-auto inline-flex items-center justify-center gap-2 px-4 py-2 sm:px-12 sm:py-2.5 bg-[#0d6efd] rounded-lg text-white font-medium hover:opacity-70 transition-opacity order-1"
        >
          <span>Dashboard</span>
          <svg
            aria-hidden={true}
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="M5 12h14M12 5l7 7-7 7" />
          </svg>
        </button>
      )}

      {showDialog &&
        createPortal(
          <div
            className="fixed inset-0 z-100 flex items-center justify-center backdrop-blur-xl bg-black/60"
            onClick={() => setShowDialog(false)}
          >
            <div
              className="bg-[#0F1115] border border-[#ffffff20] rounded-xl p-6 md:p-8 max-w-[400px] w-[calc(100%-2rem)] text-center"
              onClick={(e) => e.stopPropagation()}
            >
              <h3 className="text-lg md:text-xl font-bold mb-3">
                Cloud is in development
              </h3>
              <p className="text-gray-400 text-sm md:text-base mb-5">
                Databasus Cloud is currently under development. Stay tuned for
                updates!
              </p>
              <button
                onClick={() => setShowDialog(false)}
                className="px-6 py-2 bg-[#0d6efd] rounded-lg text-white font-medium hover:opacity-70 transition-opacity"
              >
                Close
              </button>
            </div>
          </div>,
          document.body,
        )}
    </>
  );
}
