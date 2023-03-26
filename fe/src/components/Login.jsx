import { useContext } from "react";
import { AuthContext } from "../AuthContext";
import UserForm from "./UserForm";

const Login = () => {
  const { login } = useContext(AuthContext);
  return (
    <div>
      <h1>Login</h1>
      <UserForm submit={login} />
    </div>
  );
};

export default Login;
