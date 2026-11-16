import type { ReactNode } from "react";

export function PublicSpaceMarketplaceLayout({
  summary,
  availability,
  gallery,
  summaryAside,
}: {
  summary: ReactNode;
  availability: ReactNode;
  gallery: ReactNode;
  summaryAside?: ReactNode;
}) {
  return (
    <>
      <div className="lg:grid lg:grid-cols-12 lg:items-start lg:gap-6 xl:gap-8">
        <div className="min-w-0 lg:col-span-4">
          {summary}
          {summaryAside ? <div className="mt-6">{summaryAside}</div> : null}
        </div>
        <div className="mt-8 min-w-0 lg:col-span-4 lg:mt-0">{availability}</div>
        <div className="mt-8 min-w-0 lg:col-span-4 lg:mt-0">{gallery}</div>
      </div>
    </>
  );
}
