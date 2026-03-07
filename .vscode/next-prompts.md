--
Allow specifying PATH dirs as a list at group level or root level or file level
--
The foreman web UI keeps a track of all commands where were run along with full stdio/stderr captured. If a command requires user input like password input, allow doing that through the web interface for any command.
--
Whenever I run a command using foreman, it first checks of the foreman server is running at the port. IF not, it runs the forman server as a background service - by adding this foreman to a master foreman service. Unless in foreman config, "RunInServer" config is set to false, then it runs the command there itself without running the server first.