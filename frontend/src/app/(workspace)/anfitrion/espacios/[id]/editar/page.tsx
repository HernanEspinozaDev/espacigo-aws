import { ListingEditor } from "@/features/host-listings/ui";

interface PageProps {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ tab?: string | string[] }>;
}

export default async function EditarEspacioPage({
  params,
  searchParams,
}: PageProps) {
  const [{ id }, { tab }] = await Promise.all([params, searchParams]);
  return (
    <ListingEditor
      listingId={id}
      startWithPhotos={tab === "fotos"}
      startWithLocation={tab === "ubicacion"}
      startWithContract={tab === "contrato"}
    />
  );
}
