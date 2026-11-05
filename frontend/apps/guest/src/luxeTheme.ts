// Imported first by the root layout: it switches the shared design tokens to the luxury theme before any screen or component is
// loaded. It reads the theme file directly, not the package index, so no component has been created with the classic colours yet.
import { applyLuxeTheme } from '../../../packages/ui/src/theme';

applyLuxeTheme();
