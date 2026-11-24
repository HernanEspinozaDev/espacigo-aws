import { redirect } from "next/navigation";

export default function HostsRedirectPage() {
  redirect("/como-funciona?rol=arrendador");
}
