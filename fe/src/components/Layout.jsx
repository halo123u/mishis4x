import { Outlet } from "react-router-dom";

const Layout = () => {
  return (
    <div>
      <nav>This will be a nav</nav>
      <Outlet />
    </div>
  );
};

export default Layout;
