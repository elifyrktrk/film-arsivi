/** Domain errors carry a stable code so each app can show its own translated message. */
export type DomainErrorCode =
  | "score_out_of_range"
  | "note_too_long"
  | "category_name_required"
  | "category_name_too_long"
  | "category_name_taken"
  | "category_description_too_long"
  | "category_not_found"
  | "title_not_found";

export class DomainError extends Error {
  constructor(public readonly code: DomainErrorCode) {
    super(code);
    this.name = "DomainError";
  }
}

export function isDomainError(e: unknown): e is DomainError {
  return e instanceof DomainError;
}
