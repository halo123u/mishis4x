import { Link } from "react-router-dom";

const Lobbies = () => {
  return (
    <div>
      <h1>Find or Create Lobby</h1>

      <table>
        <thead>
          <tr>
            <th>name</th>
            <th>owner</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>1 v 1</td>
            <td>waldo</td>
          </tr>
          <tr>
            <td>1 v 1</td>
            <td>waldo2</td>
          </tr>
        </tbody>
      </table>

      <div>
        <Link to={`/lobbies/create`}>Create Lobby</Link>
      </div>
    </div>
  );
};

export default Lobbies;
