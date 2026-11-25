import { ProductCard } from "@/components/product-card";
import type { PublicSpace } from "../domain/types";

export function PublicCatalogCard({
  space,
  priority = false,
  explore = false,
}: {
  space: PublicSpace;
  priority?: boolean;
  explore?: boolean;
}) {
  return (
    <ProductCard
      space={space}
      priority={priority}
      variant={explore ? "explore" : "default"}
    />
  );
}
