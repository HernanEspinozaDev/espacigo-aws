import type { RentalAvailabilityPort } from "./rental-availability.port";
import type { PublicRentalScheduleResult } from "../domain/rental-availability";

export function createRentalAvailabilityService(port: RentalAvailabilityPort) {
  return {
    getAvailability(
      spaceId: string,
      date: string,
      duration: number,
      signal?: AbortSignal,
    ) {
      return port.getAvailability(spaceId, date, duration, signal);
    },
    async getSchedule(
      spaceId: string,
      date: string,
      duration: number,
      signal?: AbortSignal,
    ): Promise<PublicRentalScheduleResult> {
      if (port.getSchedule) {
        return port.getSchedule(spaceId, date, duration, signal);
      }
      const avail = await port.getAvailability(spaceId, date, duration, signal);
      if (!avail.ok) return avail;
      return {
        ok: true,
        value: {
          space_id: avail.value.space_id,
          time_zone: avail.value.time_zone,
          rate_unit: avail.value.rate_unit,
          checked_at: avail.value.checked_at,
          slots: avail.value.items.map((item) => ({
            start_at: item.start_at,
            end_at: item.end_at,
            status: "available",
          })),
        },
      };
    },
  };
}
