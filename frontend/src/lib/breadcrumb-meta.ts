export type BreadcrumbSegment = {
  label: () => string;
  /** Pathname for intermediate crumbs; omit on the active leaf. */
  href?: string;
  /** Set false to keep the route in the tree but hide from the trail. */
  visible?: boolean;
  /**
   * Mark a leaf whose real label loads asynchronously (e.g. node name, user
   * email). The trail shows a skeleton instead of the placeholder `label()` until
   * SetBreadcrumbTitle supplies the resolved title.
   */
  dynamic?: boolean;
};

export type BreadcrumbStaticData = {
  breadcrumb?: BreadcrumbSegment;
};

export function breadcrumbStaticData(segment: BreadcrumbSegment): {
  breadcrumb: BreadcrumbSegment;
} {
  return { breadcrumb: segment };
}
