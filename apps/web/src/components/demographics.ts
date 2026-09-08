/**
 * D1: the two optional registration fields. The server validates against the
 * same closed lists (internal/server/patient_demographics.go) and returns them
 * with the patient filter options, so a value can never be introduced here that
 * the server would reject.
 */

export const civilStatusLabels: Record<string, string> = {
  single: "Single",
  married: "Married",
  common_law: "Common-law",
  divorced: "Divorced",
  separated: "Separated",
  widowed: "Widowed",
};

export const religionLabels: Record<string, string> = {
  catholic: "Catholic",
  protestant: "Protestant / Evangelical",
  baptist: "Baptist",
  adventist: "Seventh-day Adventist",
  pentecostal: "Pentecostal",
  methodist: "Methodist",
  jehovahs_witness: "Jehovah's Witness",
  vodou: "Vodou",
  muslim: "Muslim",
  jewish: "Jewish",
  none: "No religion",
  other: "Other",
  prefer_not_to_say: "Prefer not to say",
};

export const civilStatusOptions = Object.entries(civilStatusLabels).map(([value, label]) => ({ value, label }));
export const religionOptions = Object.entries(religionLabels).map(([value, label]) => ({ value, label }));

/** What to show in a profile or a printed summary; blank means not recorded. */
export function civilStatusText(value: string) {
  return civilStatusLabels[value] ?? "";
}

export function religionText(religion: string, religionOther: string) {
  if (religion === "other") return religionOther ? `Other — ${religionOther}` : "Other";
  return religionLabels[religion] ?? "";
}
