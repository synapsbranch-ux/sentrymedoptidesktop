// N6: @testing-library/jest-dom is a dependency of this project but was never
// loaded, so its matchers (toBeDisabled, toBeVisible, toHaveValue…) failed with
// "Invalid Chai property" and tests had to assert on raw DOM properties instead.
import "@testing-library/jest-dom/vitest";
