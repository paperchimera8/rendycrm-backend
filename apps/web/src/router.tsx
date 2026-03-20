import { QueryClient } from "@tanstack/react-query";
import {
  Outlet,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  redirect,
} from "@tanstack/react-router";
import { AppShell } from "./components/AppShell";
import { RouterErrorView } from "./components/RouterErrorView";
import { LoginPage } from "./routes/LoginPage";
import { CalendarPage } from "./routes/CalendarPage";
import { DashboardPage } from "./routes/DashboardPage";
import { DialogsPage } from "./routes/DialogsPage";
import { AvailabilityPage } from "./routes/AvailabilityPage";
import { ReviewsPage } from "./routes/ReviewsPage";
import { AnalyticsPage } from "./routes/AnalyticsPage";
import { SettingsPage } from "./routes/SettingsPage";
import { APP_BASE_PATH, appUrl, stripAppBasePath } from "./lib/basePath";
import { getToken } from "./lib/api";

export interface RouterContext {
  queryClient: QueryClient;
}

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: () => <Outlet />,
  errorComponent: RouterErrorView,
});

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "login",
  component: LoginPage,
  errorComponent: RouterErrorView,
});

const calendarRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "calendar",
  component: CalendarPage,
  errorComponent: RouterErrorView,
});

const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "app",
  beforeLoad: async ({ location }) => {
    if (!getToken() && stripAppBasePath(location.pathname) !== "/login") {
      throw redirect({ to: "/login" });
    }
  },
  component: AppShell,
  errorComponent: RouterErrorView,
});

const dashboardRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/",
  component: DashboardPage,
  errorComponent: RouterErrorView,
});

const dialogsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "dialogs",
  component: DialogsPage,
  errorComponent: RouterErrorView,
});

const availabilityRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "availability",
  component: AvailabilityPage,
  errorComponent: RouterErrorView,
});

const slotsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "slots",
  component: AvailabilityPage,
  errorComponent: RouterErrorView,
});

const reviewsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "reviews",
  component: ReviewsPage,
  errorComponent: RouterErrorView,
});

const analyticsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "analytics",
  component: AnalyticsPage,
  errorComponent: RouterErrorView,
});

const settingsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "settings",
  component: SettingsPage,
  errorComponent: RouterErrorView,
});

const routeTree = rootRoute.addChildren([
  loginRoute,
  calendarRoute,
  appRoute.addChildren([
    dashboardRoute,
    dialogsRoute,
    slotsRoute,
    availabilityRoute,
    reviewsRoute,
    analyticsRoute,
    settingsRoute,
  ]),
]);

export const router = createRouter({
  routeTree,
  basepath: APP_BASE_PATH,
  context: {
    queryClient: undefined as never,
  },
  defaultPreload: "intent",
  defaultErrorComponent: RouterErrorView,
  defaultNotFoundComponent: () => (
    <RouterErrorView
      error={new Error("Page not found")}
      reset={() => window.location.assign(appUrl("/"))}
      info={{ componentStack: "" }}
    />
  ),
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
