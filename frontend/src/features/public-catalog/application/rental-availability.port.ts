import type {
  PublicRentalAvailabilityResult,
  PublicRentalScheduleResult,
} from "../domain/rental-availability";

export interface RentalAvailabilityPort {
  getAvailability(
    spaceId: string,
    date: string,
    duration: number,
    signal?: AbortSignal,
  ): Promise<PublicRentalAvailabilityResult>;
  getSchedule?(
    spaceId: string,
    date: string,
    duration: number,
    signal?: AbortSignal,
  ): Promise<PublicRentalScheduleResult>;
}
