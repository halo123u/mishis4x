import { useContext } from "react";
import { useNavigate } from "react-router-dom";
import UserForm from "./UserForm";
import { AuthContext } from "../AuthContext";

const Signup = () => {
  const { updateUser } = useContext(AuthContext);
  const navigate = useNavigate();
  const createUser = (username, password) => {
    fetch("/api/user/create", {
      method: "POST",
      headers: {
        "Content-type": "application/json",
      },
      body: JSON.stringify({
        username,
        password,
      }),
    })
      .then((res) => res.json())
      .then((res) => {
        updateUser(res);
        navigate("/protected");
      })
      .catch((err) => console.log(err));
  };
  return (
    <div>
      <h1>Sign up!</h1>
      <UserForm submit={createUser} buttonText="create account" />
    </div>
  );
};

export default Signup;
