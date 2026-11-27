import type { Metadata } from "next";
import { PrivacyHubView } from "@/features/account/ui/privacy-hub-view";

export const metadata: Metadata = {
  title: "Privacidad de Datos | Mi Cuenta | EspaciGo",
  description:
    "Gestión de derechos ARCO, exportación de datos, documentos privados y revisión documental.",
  robots: {
    index: false,
    follow: false,
  },
};

export default async function PrivacidadPage({
  searchParams,
}: {
  searchParams: Promise<{ tab?: string | string[] }>;
}) {
  const { tab } = await searchParams;
  const rawTab = Array.isArray(tab) ? tab[0] : tab;
  return <PrivacyHubView initialTab={rawTab} />;
}
